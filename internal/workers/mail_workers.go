package workers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"despatch/internal/config"
	"despatch/internal/mail"
	mailsecclient "despatch/internal/mailsec"
	"despatch/internal/models"
	"despatch/internal/store"
	"despatch/internal/util"
)

type MailWorkers struct {
	cfg        config.Config
	st         *store.Store
	encryptKey []byte
}

func StartMailWorkers(ctx context.Context, cfg config.Config, st *store.Store) {
	w := &MailWorkers{
		cfg:        cfg,
		st:         st,
		encryptKey: util.Derive32ByteKey(cfg.SessionEncryptKey),
	}
	go w.runSyncLoop(ctx)
	go w.runScheduledSendLoop(ctx)
}

func (w *MailWorkers) runSyncLoop(ctx context.Context) {
	w.syncAllAccounts(ctx)
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.syncAllAccounts(ctx)
		}
	}
}

func (w *MailWorkers) syncAllAccounts(ctx context.Context) {
	accounts, err := w.st.ListAllMailAccounts(ctx)
	if err != nil {
		log.Printf("mail_sync list_accounts_failed error=%v", err)
		return
	}
	for _, account := range accounts {
		accountCtx, cancel := context.WithTimeout(ctx, 55*time.Second)
		err := w.syncAccount(accountCtx, account)
		cancel()
		if err != nil {
			log.Printf("mail_sync account=%s status=error error=%v", account.ID, err)
			_ = w.st.UpdateMailAccountSyncStatus(ctx, account.ID, time.Now().UTC(), err.Error())
			continue
		}
		_ = w.st.UpdateMailAccountSyncStatus(ctx, account.ID, time.Now().UTC(), "")
	}
}

func (w *MailWorkers) syncAccount(ctx context.Context, account models.MailAccount) error {
	if strings.TrimSpace(account.SecretEnc) == "" || strings.TrimSpace(account.Login) == "" {
		return errors.New("missing account credentials")
	}
	pass, err := util.DecryptString(w.encryptKey, account.SecretEnc)
	if err != nil {
		return err
	}
	cli := mail.NewIMAPSMTPClient(w.accountMailConfig(account))
	mailboxes, err := cli.ListMailboxes(ctx, account.Login, pass)
	if err != nil {
		return err
	}
	for _, mailbox := range mailboxes {
		name := strings.TrimSpace(mailbox.Name)
		if name == "" {
			continue
		}
		summaries, err := cli.ListMessages(ctx, account.Login, pass, name, 1, 200)
		if err != nil {
			log.Printf("mail_sync account=%s mailbox=%s list_messages_failed error=%v", account.ID, name, err)
			continue
		}
		for _, summary := range summaries {
			msg, err := cli.GetMessage(ctx, account.Login, pass, summary.ID)
			if err != nil {
				log.Printf("mail_sync account=%s message=%s get_failed error=%v", account.ID, summary.ID, err)
				continue
			}
			scopedMessageID := mail.NormalizeIndexedMessageID(account.ID, summary.ID)
			threadID := deriveThreadID(name, msg.Subject, msg.From)
			now := time.Now().UTC()
			bodyText := strings.TrimSpace(msg.Body)
			bodyHTMLSanitized := ""
			rawSource := bodyText
			fromValue := firstNonEmptyString(msg.From, summary.From)
			toValue := strings.Join(msg.To, ", ")
			subject := firstNonEmptyString(msg.Subject, summary.Subject)
			snippetValue := snippet(bodyText, 180)
			dkimStatus := "unknown"
			spfStatus := "unknown"
			dmarcStatus := "unknown"
			phishingScore := 0.0
			remoteImagesBlocked := true
			indexedAttachments := convertMessageAttachments(account.ID, scopedMessageID, msg.Attachments, now)
			hasAttachments := len(indexedAttachments) > 0

			if w.cfg.MailSecEnabled {
				rawMessage, rawErr := cli.GetRawMessage(ctx, account.Login, pass, summary.ID)
				if rawErr != nil {
					log.Printf("mail_sync account=%s message=%s mailsec_raw_failed error=%v", account.ID, summary.ID, rawErr)
				} else {
					analysis, analysisErr := w.parseAndClassifyWithMailSec(ctx, account.ID, scopedMessageID, rawMessage, now)
					if analysisErr != nil {
						log.Printf("mail_sync account=%s message=%s mailsec_parse_failed error=%v", account.ID, summary.ID, analysisErr)
					} else {
						if v := strings.TrimSpace(analysis.Subject); v != "" {
							subject = v
						}
						if v := strings.TrimSpace(analysis.FromValue); v != "" {
							fromValue = v
						}
						if v := strings.TrimSpace(analysis.ToValue); v != "" {
							toValue = v
						}
						if v := strings.TrimSpace(analysis.BodyText); v != "" {
							bodyText = v
						}
						if v := strings.TrimSpace(analysis.Snippet); v != "" {
							snippetValue = v
						} else {
							snippetValue = snippet(bodyText, 180)
						}
						bodyHTMLSanitized = analysis.BodyHTMLSanitized
						rawSource = analysis.RawSource
						dkimStatus = analysis.DKIMStatus
						spfStatus = analysis.SPFStatus
						dmarcStatus = analysis.DMARCStatus
						phishingScore = analysis.PhishingScore
						remoteImagesBlocked = analysis.RemoteImagesBlocked
						if len(analysis.Attachments) > 0 {
							indexedAttachments = analysis.Attachments
						}
						hasAttachments = hasAttachments || analysis.HasAttachments || len(indexedAttachments) > 0
						threadID = deriveThreadID(firstNonEmptyString(msg.Mailbox, name), subject, fromValue)
					}
				}
			}
			scopedThreadID := mail.NormalizeIndexedThreadID(account.ID, threadID)

			indexed := models.IndexedMessage{
				ID:                  scopedMessageID,
				AccountID:           account.ID,
				Mailbox:             firstNonEmptyString(msg.Mailbox, name),
				UID:                 msg.UID,
				ThreadID:            scopedThreadID,
				FromValue:           fromValue,
				ToValue:             toValue,
				Subject:             subject,
				Snippet:             snippetValue,
				BodyText:            bodyText,
				BodyHTMLSanitized:   bodyHTMLSanitized,
				RawSource:           rawSource,
				Seen:                summary.Seen,
				Flagged:             false,
				Answered:            false,
				Draft:               false,
				HasAttachments:      hasAttachments,
				Importance:          0,
				DKIMStatus:          dkimStatus,
				SPFStatus:           spfStatus,
				DMARCStatus:         dmarcStatus,
				PhishingScore:       phishingScore,
				RemoteImagesBlocked: remoteImagesBlocked,
				RemoteImagesAllowed: false,
				DateHeader:          chooseTime(msg.Date, summary.Date, now),
				InternalDate:        chooseTime(summary.Date, msg.Date, now),
			}
			if _, err := w.st.UpsertIndexedMessage(ctx, indexed); err != nil {
				log.Printf("mail_sync account=%s message=%s index_upsert_failed error=%v", account.ID, summary.ID, err)
				continue
			}
			if err := w.st.ReplaceIndexedAttachments(ctx, account.ID, scopedMessageID, indexedAttachments); err != nil {
				log.Printf("mail_sync account=%s message=%s attachments_upsert_failed error=%v", account.ID, summary.ID, err)
			}
		}
	}
	return w.st.RebuildThreadIndex(ctx, account.ID)
}

func (w *MailWorkers) runScheduledSendLoop(ctx context.Context) {
	w.processScheduledSends(ctx)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processScheduledSends(ctx)
		}
	}
}

func (w *MailWorkers) processScheduledSends(ctx context.Context) {
	items, err := w.st.ListDueScheduledSends(ctx, time.Now().UTC(), 50)
	if err != nil {
		log.Printf("scheduled_send list_due_failed error=%v", err)
		return
	}
	for _, item := range items {
		if err := w.processScheduledSendItem(ctx, item); err != nil {
			log.Printf("scheduled_send queue_id=%s error=%v", item.ID, err)
		}
	}
}

func (w *MailWorkers) processScheduledSendItem(ctx context.Context, item models.ScheduledSendQueueItem) error {
	draft, err := w.st.GetDraftByID(ctx, item.UserID, item.DraftID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = w.st.MarkScheduledSendFailed(ctx, item.ID, "draft not found")
			return nil
		}
		return err
	}
	account, err := w.st.GetMailAccountByID(ctx, item.UserID, item.AccountID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = w.st.MarkScheduledSendFailed(ctx, item.ID, "mail account not found")
			_ = w.st.SetDraftStatus(ctx, item.UserID, item.DraftID, "failed")
			return nil
		}
		return err
	}
	user, err := w.st.GetUserByID(ctx, item.UserID)
	if err != nil {
		return err
	}
	pass, err := util.DecryptString(w.encryptKey, account.SecretEnc)
	if err != nil {
		return err
	}
	recipients := append(splitCSV(draft.ToValue), splitCSV(draft.CCValue)...)
	recipients = append(recipients, splitCSV(draft.BCCValue)...)
	if len(recipients) == 0 {
		_ = w.st.MarkScheduledSendFailed(ctx, item.ID, "no recipients")
		_ = w.st.SetDraftStatus(ctx, item.UserID, item.DraftID, "failed")
		return nil
	}

	sendReq := mail.SendRequest{
		From:    user.Email,
		To:      recipients,
		Subject: draft.Subject,
		Body:    draft.BodyText,
	}
	cli := mail.NewIMAPSMTPClient(w.accountMailConfig(account))
	if _, err := cli.Send(ctx, account.Login, pass, sendReq); err != nil {
		retryCount := item.RetryCount + 1
		if retryCount >= 3 {
			_ = w.st.MarkScheduledSendFailed(ctx, item.ID, err.Error())
			_ = w.st.SetDraftStatus(ctx, item.UserID, item.DraftID, "failed")
			return nil
		}
		nextRetry := time.Now().UTC().Add(time.Duration(1<<uint(retryCount-1)) * 30 * time.Second)
		_ = w.st.MarkScheduledSendRetry(ctx, item.ID, retryCount, nextRetry, err.Error())
		_ = w.st.SetDraftStatus(ctx, item.UserID, item.DraftID, "retrying")
		return nil
	}

	_ = w.st.MarkScheduledSendSent(ctx, item.ID)
	_ = w.st.SetDraftStatus(ctx, item.UserID, item.DraftID, "sent")
	return nil
}

func (w *MailWorkers) accountMailConfig(account models.MailAccount) config.Config {
	cfg := w.cfg
	cfg.IMAPHost = account.IMAPHost
	cfg.IMAPPort = account.IMAPPort
	cfg.IMAPTLS = account.IMAPTLS
	cfg.IMAPStartTLS = account.IMAPStartTLS
	cfg.SMTPHost = account.SMTPHost
	cfg.SMTPPort = account.SMTPPort
	cfg.SMTPTLS = account.SMTPTLS
	cfg.SMTPStartTLS = account.SMTPStartTLS
	return cfg
}

func deriveThreadID(mailbox, subject, from string) string {
	normalized := strings.ToLower(strings.TrimSpace(subject))
	for {
		switch {
		case strings.HasPrefix(normalized, "re:"):
			normalized = strings.TrimSpace(strings.TrimPrefix(normalized, "re:"))
		case strings.HasPrefix(normalized, "fwd:"):
			normalized = strings.TrimSpace(strings.TrimPrefix(normalized, "fwd:"))
		case strings.HasPrefix(normalized, "fw:"):
			normalized = strings.TrimSpace(strings.TrimPrefix(normalized, "fw:"))
		default:
			goto done
		}
	}
done:
	if normalized == "" {
		normalized = strings.ToLower(strings.TrimSpace(from))
	}
	if normalized == "" {
		normalized = "untitled"
	}
	base := strings.ToLower(strings.TrimSpace(mailbox)) + "\x00" + normalized
	sum := sha256.Sum256([]byte(base))
	return strings.ToLower(strings.TrimSpace(mailbox)) + ":" + hex.EncodeToString(sum[:10])
}

func snippet(body string, max int) string {
	if max <= 0 {
		max = 180
	}
	compact := strings.Join(strings.Fields(body), " ")
	if len(compact) <= max {
		return compact
	}
	return compact[:max]
}

func chooseTime(values ...time.Time) time.Time {
	for _, v := range values {
		if !v.IsZero() {
			return v.UTC()
		}
	}
	return time.Now().UTC()
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type mailsecMessageAnalysis struct {
	Subject             string
	FromValue           string
	ToValue             string
	Snippet             string
	BodyText            string
	BodyHTMLSanitized   string
	RawSource           string
	HasAttachments      bool
	Attachments         []models.IndexedAttachment
	DKIMStatus          string
	SPFStatus           string
	DMARCStatus         string
	PhishingScore       float64
	RemoteImagesBlocked bool
}

func convertMessageAttachments(accountID, messageID string, source []mail.AttachmentMeta, now time.Time) []models.IndexedAttachment {
	out := make([]models.IndexedAttachment, 0, len(source))
	for _, a := range source {
		out = append(out, models.IndexedAttachment{
			ID:          a.ID,
			MessageID:   messageID,
			AccountID:   accountID,
			Filename:    strings.TrimSpace(a.Filename),
			ContentType: firstNonEmptyString(strings.TrimSpace(a.ContentType), "application/octet-stream"),
			SizeBytes:   a.Size,
			InlinePart:  false,
			CreatedAt:   now,
		})
	}
	return out
}

func (w *MailWorkers) parseAndClassifyWithMailSec(ctx context.Context, accountID, messageID string, raw []byte, now time.Time) (mailsecMessageAnalysis, error) {
	rawB64 := base64.RawURLEncoding.EncodeToString(raw)
	parsedResult, err := w.callMailSec(ctx, accountID, messageID, "mime.parse", map[string]any{
		"raw_b64url": rawB64,
	})
	if err != nil {
		return mailsecMessageAnalysis{}, err
	}

	out := mailsecMessageAnalysis{
		Subject:             getMapString(parsedResult, "subject"),
		FromValue:           getMapString(parsedResult, "from"),
		ToValue:             getMapString(parsedResult, "to"),
		Snippet:             getMapString(parsedResult, "snippet"),
		BodyText:            getMapString(parsedResult, "body_text"),
		RawSource:           rawPreview(raw, 1<<20),
		DKIMStatus:          "unknown",
		SPFStatus:           "unknown",
		DMARCStatus:         "unknown",
		RemoteImagesBlocked: true,
	}
	if out.RawSource == "" {
		out.RawSource = out.BodyText
	}

	parsedAttachments := parseMailSecAttachments(accountID, messageID, getMapAnySlice(parsedResult, "attachments"), now)
	if len(parsedAttachments) > 0 {
		out.Attachments = parsedAttachments
		out.HasAttachments = true
	}
	if hasAttachments, ok := getMapBool(parsedResult, "has_attachments"); ok {
		out.HasAttachments = out.HasAttachments || hasAttachments
	}

	if rawHTML := strings.TrimSpace(getMapString(parsedResult, "body_html")); rawHTML != "" {
		sanitizedResult, sanitizeErr := w.callMailSec(ctx, accountID, messageID, "html.sanitize", map[string]any{
			"html":                rawHTML,
			"allow_remote_images": false,
		})
		if sanitizeErr == nil {
			out.BodyHTMLSanitized = getMapString(sanitizedResult, "html")
			if blocked, ok := getMapBool(sanitizedResult, "remote_images_blocked"); ok {
				out.RemoteImagesBlocked = blocked
			}
		} else {
			log.Printf("mail_sync account=%s message=%s mailsec_html_sanitize_failed error=%v", accountID, messageID, sanitizeErr)
		}
	}

	authResult, authErr := w.callMailSec(ctx, accountID, messageID, "auth.verify", map[string]any{
		"raw_b64url": rawB64,
	})
	if authErr != nil {
		log.Printf("mail_sync account=%s message=%s mailsec_auth_verify_failed error=%v", accountID, messageID, authErr)
		return out, nil
	}

	out.DKIMStatus = firstNonEmptyString(strings.TrimSpace(getMapString(authResult, "dkim")), "unknown")
	out.SPFStatus = firstNonEmptyString(strings.TrimSpace(getMapString(authResult, "spf")), "unknown")
	out.DMARCStatus = firstNonEmptyString(strings.TrimSpace(getMapString(authResult, "dmarc")), "unknown")
	if score, ok := getMapFloat64(authResult, "phishing_score"); ok {
		out.PhishingScore = score
	}

	return out, nil
}

func (w *MailWorkers) callMailSec(ctx context.Context, accountID, messageID, op string, payload map[string]any) (map[string]any, error) {
	if !w.cfg.MailSecEnabled {
		return nil, fmt.Errorf("mailsec is disabled")
	}
	timeout := time.Duration(w.cfg.MailSecTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cli := mailsecclient.NewClient(w.cfg.MailSecSocket)
	resp, err := cli.Call(callCtx, mailsecclient.Request{
		RequestID:  fmt.Sprintf("mailsec-sync-%d", time.Now().UnixNano()),
		Op:         strings.TrimSpace(op),
		AccountID:  firstNonEmptyString(strings.TrimSpace(accountID), "sync"),
		MessageID:  firstNonEmptyString(strings.TrimSpace(messageID), fmt.Sprintf("msg-%d", time.Now().UnixNano())),
		Payload:    payload,
		DeadlineMS: int(timeout.Milliseconds()),
	})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("%s: %s", strings.TrimSpace(resp.Code), strings.TrimSpace(resp.Error))
	}
	if resp.Result == nil {
		return map[string]any{}, nil
	}
	return resp.Result, nil
}

func parseMailSecAttachments(accountID, messageID string, values []any, now time.Time) []models.IndexedAttachment {
	out := make([]models.IndexedAttachment, 0, len(values))
	for i, item := range values {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := strings.TrimSpace(getMapString(row, "id"))
		if id == "" {
			id = messageID + ":part:" + strconv.Itoa(i+1)
		}
		size, _ := getMapInt64(row, "size_bytes")
		inline, _ := getMapBool(row, "inline")
		out = append(out, models.IndexedAttachment{
			ID:          id,
			MessageID:   messageID,
			AccountID:   accountID,
			Filename:    strings.TrimSpace(getMapString(row, "filename")),
			ContentType: firstNonEmptyString(strings.TrimSpace(getMapString(row, "content_type")), "application/octet-stream"),
			SizeBytes:   size,
			InlinePart:  inline,
			CreatedAt:   now,
		})
	}
	return out
}

func rawPreview(raw []byte, max int) string {
	if len(raw) == 0 {
		return ""
	}
	if max <= 0 || len(raw) <= max {
		return string(raw)
	}
	return string(raw[:max])
}

func getMapAnySlice(m map[string]any, key string) []any {
	v, ok := m[key]
	if !ok {
		return nil
	}
	out, ok := v.([]any)
	if !ok {
		return nil
	}
	return out
}

func getMapString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func getMapBool(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	switch vv := v.(type) {
	case bool:
		return vv, true
	case string:
		switch strings.ToLower(strings.TrimSpace(vv)) {
		case "true":
			return true, true
		case "false":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func getMapFloat64(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch vv := v.(type) {
	case float64:
		return vv, true
	case float32:
		return float64(vv), true
	case int:
		return float64(vv), true
	case int64:
		return float64(vv), true
	default:
		return 0, false
	}
}

func getMapInt64(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch vv := v.(type) {
	case int:
		return int64(vv), true
	case int64:
		return vv, true
	case float64:
		return int64(vv), true
	default:
		return 0, false
	}
}
