package mail

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"io/fs"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	stdmail "net/mail"
	"net/textproto"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	imapquota "github.com/emersion/go-imap-quota"
	imapclient "github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"

	"despatch/internal/config"
)

const (
	defaultDialTimeout = 10 * time.Second
	maxBodyBytes       = 1 << 20  // 1 MiB body preview
	maxAttachmentBytes = 25 << 20 // 25 MiB per attachment
	previewSampleBytes = 8192
)

type IMAPSMTPClient struct {
	cfg config.Config
}

func NewIMAPSMTPClient(cfg config.Config) *IMAPSMTPClient {
	return &IMAPSMTPClient{cfg: cfg}
}

func (c *IMAPSMTPClient) ListMailboxes(ctx context.Context, user, pass string) ([]Mailbox, error) {
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	return c.listMailboxesWithClient(cli, true)
}

func (c *IMAPSMTPClient) CreateMailbox(ctx context.Context, user, pass, mailbox string) error {
	name := strings.TrimSpace(mailbox)
	if name == "" {
		return fmt.Errorf("mailbox name is required")
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return err
	}
	defer cli.Logout()
	return cli.Create(name)
}

func (c *IMAPSMTPClient) RenameMailbox(ctx context.Context, user, pass, mailbox, newMailbox string) error {
	name := strings.TrimSpace(mailbox)
	target := strings.TrimSpace(newMailbox)
	if name == "" {
		return fmt.Errorf("mailbox name is required")
	}
	if target == "" {
		return fmt.Errorf("new mailbox name is required")
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return err
	}
	defer cli.Logout()
	return cli.Rename(name, target)
}

func (c *IMAPSMTPClient) DeleteMailbox(ctx context.Context, user, pass, mailbox string) error {
	name := strings.TrimSpace(mailbox)
	if name == "" {
		return fmt.Errorf("mailbox name is required")
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return err
	}
	defer cli.Logout()
	return cli.Delete(name)
}

func (c *IMAPSMTPClient) GetQuota(ctx context.Context, user, pass string) (Quota, error) {
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return Quota{}, err
	}
	defer cli.Logout()

	quotaClient := imapquota.NewClient(cli)
	supported, err := quotaClient.SupportQuota()
	if err != nil {
		return Quota{}, err
	}
	if !supported {
		return Quota{}, ErrQuotaUnsupported
	}
	statuses, err := quotaClient.GetQuotaRoot("INBOX")
	if err != nil {
		return Quota{}, err
	}
	var out Quota
	for _, status := range statuses {
		if status == nil {
			continue
		}
		for name, usage := range status.Resources {
			switch strings.ToUpper(strings.TrimSpace(name)) {
			case "STORAGE":
				used := int64(usage[0]) * 1024
				total := int64(usage[1]) * 1024
				if used > out.UsedBytes {
					out.UsedBytes = used
				}
				if total > out.TotalBytes {
					out.TotalBytes = total
				}
			case "MESSAGE":
				if int64(usage[0]) > out.UsedMessages {
					out.UsedMessages = int64(usage[0])
				}
				if int64(usage[1]) > out.TotalMessages {
					out.TotalMessages = int64(usage[1])
				}
			}
		}
	}
	return out, nil
}

func (c *IMAPSMTPClient) listMailboxesWithClient(cli *imapclient.Client, withStatus bool) ([]Mailbox, error) {
	ch := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() {
		done <- cli.List("", "*", ch)
	}()

	var out []Mailbox
	for mb := range ch {
		item := Mailbox{Name: mb.Name, Role: MailboxRole(mb.Name, mb.Attributes)}
		if withStatus {
			status, err := cli.Status(mb.Name, []imap.StatusItem{imap.StatusMessages, imap.StatusUnseen})
			if err != nil {
				continue
			}
			item.Unread = int(status.Unseen)
			item.Messages = int(status.Messages)
		}
		out = append(out, item)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	if len(out) == 0 {
		out = []Mailbox{{Name: "INBOX", Role: "inbox"}}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Role == "inbox" || out[i].Name == "INBOX" {
			return true
		}
		if out[j].Role == "inbox" || out[j].Name == "INBOX" {
			return false
		}
		if out[i].Role != out[j].Role {
			return out[i].Role < out[j].Role
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (c *IMAPSMTPClient) ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]MessageSummary, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	mbox, err := cli.Select(mailbox, true)
	if err != nil {
		return nil, err
	}
	if mbox.Messages == 0 {
		return []MessageSummary{}, nil
	}
	uids, err := cli.UidSearch(imap.NewSearchCriteria())
	if err != nil {
		return nil, err
	}
	selected := mailboxPageUIDs(uids, page, pageSize)
	if len(selected) == 0 {
		return []MessageSummary{}, nil
	}
	return fetchLiveMessageSummariesByUIDs(cli, mailbox, selected)
}

func (c *IMAPSMTPClient) GetMessage(ctx context.Context, user, pass, id string) (Message, error) {
	mailbox, uid, err := DecodeMessageID(id)
	if err != nil {
		return Message{}, err
	}
	raw, env, flags, err := c.fetchRawMessage(ctx, user, pass, mailbox, uid)
	if err != nil {
		return Message{}, err
	}
	parsed, err := parseMessage(raw, id, mailbox, uid)
	if err != nil {
		return Message{}, err
	}
	if parsed.Subject == "" && env != nil {
		parsed.Subject = DecodeHeaderText(env.Subject)
	}
	if parsed.From == "" && env != nil {
		parsed.From = envelopeFirstAddress(env.From)
	}
	if parsed.Date.IsZero() && env != nil {
		parsed.Date = env.Date
	}
	parsed.Seen = hasFlag(flags, imap.SeenFlag)
	parsed.Flagged = hasFlag(flags, imap.FlaggedFlag)
	parsed.Answered = hasFlag(flags, imap.AnsweredFlag)
	PopulateLiveMessageThreadID(&parsed)
	return parsed, nil
}

func (c *IMAPSMTPClient) GetRawMessage(ctx context.Context, user, pass, id string) ([]byte, error) {
	mailbox, uid, err := DecodeMessageID(id)
	if err != nil {
		return nil, err
	}
	raw, _, _, err := c.fetchRawMessage(ctx, user, pass, mailbox, uid)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *IMAPSMTPClient) Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]MessageSummary, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	if strings.TrimSpace(mailbox) == "" {
		mailbox = "INBOX"
	}
	if _, err := cli.Select(mailbox, true); err != nil {
		return nil, err
	}
	criteria := imap.NewSearchCriteria()
	if strings.TrimSpace(query) != "" {
		criteria.Text = []string{query}
	}
	uids, err := cli.UidSearch(criteria)
	if err != nil {
		return nil, err
	}
	selected := mailboxPageUIDs(uids, page, pageSize)
	if len(selected) == 0 {
		return []MessageSummary{}, nil
	}
	return fetchLiveMessageSummariesByUIDs(cli, mailbox, selected)
}

func mailboxPageUIDs(uids []uint32, page, pageSize int) []uint32 {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 25
	}
	ordered := uniqueUIDsDescending(uids)
	if len(ordered) == 0 {
		return nil
	}
	start := (page - 1) * pageSize
	if start >= len(ordered) {
		return nil
	}
	end := start + pageSize
	if end > len(ordered) {
		end = len(ordered)
	}
	return append([]uint32(nil), ordered[start:end]...)
}

func uniqueUIDsDescending(uids []uint32) []uint32 {
	if len(uids) == 0 {
		return nil
	}
	ordered := append([]uint32(nil), uids...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] > ordered[j] })
	out := make([]uint32, 0, len(ordered))
	var last uint32
	for i, uid := range ordered {
		if uid == 0 {
			continue
		}
		if i > 0 && uid == last {
			continue
		}
		out = append(out, uid)
		last = uid
	}
	return out
}

func fetchLiveMessageSummariesByUIDs(cli *imapclient.Client, mailbox string, selected []uint32) ([]MessageSummary, error) {
	if len(selected) == 0 {
		return []MessageSummary{}, nil
	}
	seq := new(imap.SeqSet)
	for _, u := range selected {
		seq.AddNum(u)
	}
	previewSection := &imap.BodySectionName{
		Peek:    true,
		Partial: []int{0, previewSampleBytes},
	}
	threadHeaderSection := &imap.BodySectionName{
		Peek: true,
		BodyPartName: imap.BodyPartName{
			Specifier: imap.HeaderSpecifier,
			Fields:    []string{"Message-Id", "In-Reply-To", "References"},
		},
	}
	items := []imap.FetchItem{
		imap.FetchEnvelope,
		imap.FetchFlags,
		imap.FetchUid,
		imap.FetchInternalDate,
		previewSection.FetchItem(),
		threadHeaderSection.FetchItem(),
	}
	messages := make(chan *imap.Message, len(selected))
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(seq, items, messages)
	}()

	msgByUID := collectFetchedMessageSummariesByUID(messages, mailbox, previewSection, threadHeaderSection)
	if err := <-done; err != nil {
		return nil, err
	}
	return orderedMessageSummariesForUIDs(selected, msgByUID), nil
}

func collectFetchedMessageSummariesByUID(messages <-chan *imap.Message, mailbox string, previewSection, threadHeaderSection *imap.BodySectionName) map[uint32]MessageSummary {
	msgByUID := make(map[uint32]MessageSummary)
	for msg := range messages {
		if msg == nil || msg.Uid == 0 {
			continue
		}
		if _, exists := msgByUID[msg.Uid]; exists {
			continue
		}
		from := ""
		subject := ""
		if msg.Envelope != nil {
			from = envelopeFirstAddress(msg.Envelope.From)
			subject = DecodeHeaderText(msg.Envelope.Subject)
		}
		s := buildMessageSummary(msg, mailbox, from, subject, msg.InternalDate, previewSection, threadHeaderSection)
		if msg.Envelope != nil && !msg.Envelope.Date.IsZero() {
			s.Date = msg.Envelope.Date
		}
		msgByUID[msg.Uid] = s
	}
	return msgByUID
}

func orderedMessageSummariesForUIDs(selected []uint32, msgByUID map[uint32]MessageSummary) []MessageSummary {
	out := make([]MessageSummary, 0, len(selected))
	seen := make(map[uint32]struct{}, len(selected))
	for _, uid := range selected {
		if uid == 0 {
			continue
		}
		if _, exists := seen[uid]; exists {
			continue
		}
		seen[uid] = struct{}{}
		if summary, ok := msgByUID[uid]; ok {
			out = append(out, summary)
		}
	}
	return out
}

func (c *IMAPSMTPClient) Send(ctx context.Context, user, pass string, req SendRequest) (SendResult, error) {
	req.To = normalizedRecipients(req.To)
	req.CC = normalizedRecipients(req.CC)
	req.BCC = normalizedRecipients(req.BCC)
	if len(mergeRecipients(req.To, req.CC, req.BCC)) == 0 {
		return SendResult{}, fmt.Errorf("at least one recipient is required")
	}
	req.HeaderFromEmail = firstNonEmptyTrimmed(req.HeaderFromEmail, req.From, user)
	req.EnvelopeFrom = firstNonEmptyTrimmed(req.EnvelopeFrom, req.HeaderFromEmail, user)

	msg, err := buildRFC822(req)
	if err != nil {
		return SendResult{}, err
	}

	if err := c.sendWithSenderFallback(ctx, user, pass, req, msg, c.sendSMTP); err != nil {
		return SendResult{}, err
	}
	return c.appendSentCopy(ctx, user, pass, msg, req.SentMailbox)
}

func (c *IMAPSMTPClient) sendWithSenderFallback(
	ctx context.Context,
	user,
	pass string,
	req SendRequest,
	raw []byte,
	sendFunc func(context.Context, string, string, string, []string, []byte) error,
) error {
	allRecipients := mergeRecipients(req.To, req.CC, req.BCC)
	if len(allRecipients) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	envelopeFrom := strings.TrimSpace(req.EnvelopeFrom)
	if envelopeFrom == "" {
		envelopeFrom = firstNonEmptyTrimmed(req.HeaderFromEmail, req.From, user)
	}
	err := sendFunc(ctx, user, pass, envelopeFrom, allRecipients, raw)
	if err == nil {
		return nil
	}
	if !IsSMTPSenderPolicyError(err) {
		return err
	}
	authIdentity := strings.TrimSpace(user)
	if authIdentity != "" && !strings.EqualFold(envelopeFrom, authIdentity) {
		retryErr := sendFunc(ctx, user, pass, authIdentity, allRecipients, raw)
		if retryErr == nil {
			return nil
		}
		if IsSMTPSenderPolicyError(retryErr) {
			return WrapSMTPSenderRejected(retryErr)
		}
		return retryErr
	}
	return WrapSMTPSenderRejected(err)
}

func (c *IMAPSMTPClient) appendSentCopy(ctx context.Context, user, pass string, raw []byte, preferredMailbox string) (SendResult, error) {
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return SendResult{SavedCopy: false, Warning: fmt.Sprintf("Message sent, but a Sent copy could not be saved: %v", err)}, nil
	}
	defer cli.Logout()

	mailboxes, err := c.listMailboxesWithClient(cli, false)
	if err != nil {
		return SendResult{SavedCopy: false, Warning: fmt.Sprintf("Message sent, but a Sent copy could not be saved: %v", err)}, nil
	}
	sentMailbox := strings.TrimSpace(preferredMailbox)
	if sentMailbox != "" {
		matched := ""
		for _, mailbox := range mailboxes {
			if strings.EqualFold(strings.TrimSpace(mailbox.Name), sentMailbox) {
				matched = strings.TrimSpace(mailbox.Name)
				break
			}
		}
		sentMailbox = matched
	}
	if sentMailbox == "" {
		sentMailbox = ResolveMailboxByRole(mailboxes, "sent")
	}
	if sentMailbox == "" {
		return SendResult{SavedCopy: false, Warning: "Message sent, but no Sent mailbox could be found to save a copy."}, nil
	}
	if err := cli.Append(sentMailbox, []string{imap.SeenFlag}, time.Now().UTC(), bytes.NewReader(raw)); err != nil {
		return SendResult{
			SavedCopy:        false,
			SavedCopyMailbox: sentMailbox,
			Warning:          fmt.Sprintf("Message sent, but the Sent copy could not be saved to %s: %v", sentMailbox, err),
		}, nil
	}
	return SendResult{SavedCopy: true, SavedCopyMailbox: sentMailbox}, nil
}

func (c *IMAPSMTPClient) SetFlags(ctx context.Context, user, pass, id string, flags []string) error {
	mailbox, uid, err := DecodeMessageID(id)
	if err != nil {
		return err
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return err
	}
	defer cli.Logout()
	if _, err := cli.Select(mailbox, false); err != nil {
		return err
	}
	seq := new(imap.SeqSet)
	seq.AddNum(uid)

	values := make([]interface{}, 0, len(flags))
	for _, f := range flags {
		values = append(values, canonicalFlag(f))
	}
	item := imap.FormatFlagsOp(imap.SetFlags, true)
	return cli.UidStore(seq, item, values, nil)
}

func (c *IMAPSMTPClient) UpdateFlags(ctx context.Context, user, pass, id string, patch FlagPatch) error {
	mailbox, uid, err := DecodeMessageID(id)
	if err != nil {
		return err
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return err
	}
	defer cli.Logout()
	if _, err := cli.Select(mailbox, false); err != nil {
		return err
	}
	seq := new(imap.SeqSet)
	seq.AddNum(uid)

	if len(patch.Add) > 0 {
		values := make([]interface{}, 0, len(patch.Add))
		for _, f := range patch.Add {
			values = append(values, canonicalFlag(f))
		}
		item := imap.FormatFlagsOp(imap.AddFlags, true)
		if err := cli.UidStore(seq, item, values, nil); err != nil {
			return err
		}
	}
	if len(patch.Remove) > 0 {
		values := make([]interface{}, 0, len(patch.Remove))
		for _, f := range patch.Remove {
			values = append(values, canonicalFlag(f))
		}
		item := imap.FormatFlagsOp(imap.RemoveFlags, true)
		if err := cli.UidStore(seq, item, values, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *IMAPSMTPClient) Move(ctx context.Context, user, pass, id, mailbox string) error {
	srcMbox, uid, err := DecodeMessageID(id)
	if err != nil {
		return err
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return err
	}
	defer cli.Logout()
	if _, err := cli.Select(srcMbox, false); err != nil {
		return err
	}
	seq := new(imap.SeqSet)
	seq.AddNum(uid)
	return cli.UidMove(seq, mailbox)
}

func (c *IMAPSMTPClient) GetAttachment(ctx context.Context, user, pass, attachmentID string) (AttachmentContent, error) {
	meta, rc, err := c.GetAttachmentStream(ctx, user, pass, attachmentID)
	if err != nil {
		return AttachmentContent{}, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return AttachmentContent{}, err
	}
	return AttachmentContent{Meta: meta, Data: data}, nil
}

func (c *IMAPSMTPClient) GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (AttachmentMeta, io.ReadCloser, error) {
	msgID, part, err := DecodeAttachmentID(attachmentID)
	if err != nil {
		return AttachmentMeta{}, nil, err
	}
	mailbox, uid, err := DecodeMessageID(msgID)
	if err != nil {
		return AttachmentMeta{}, nil, err
	}
	raw, _, _, err := c.fetchRawMessage(ctx, user, pass, mailbox, uid)
	if err != nil {
		return AttachmentMeta{}, nil, err
	}
	meta, data, err := extractAttachment(raw, msgID, part)
	if err != nil {
		return AttachmentMeta{}, nil, err
	}
	return meta, io.NopCloser(bytes.NewReader(data)), nil
}

func (c *IMAPSMTPClient) fetchRawMessage(ctx context.Context, user, pass, mailbox string, uid uint32) ([]byte, *imap.Envelope, []string, error) {
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, nil, nil, err
	}
	defer cli.Logout()
	if _, err := cli.Select(mailbox, true); err != nil {
		return nil, nil, nil, err
	}
	seq := new(imap.SeqSet)
	seq.AddNum(uid)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchEnvelope, imap.FetchFlags, section.FetchItem()}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(seq, items, messages)
	}()
	msg := <-messages
	if err := <-done; err != nil {
		return nil, nil, nil, err
	}
	if msg == nil {
		return nil, nil, nil, fmt.Errorf("message not found")
	}
	body := msg.GetBody(section)
	if body == nil {
		return nil, nil, nil, fmt.Errorf("server did not return message body")
	}
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, nil, nil, err
	}
	return raw, msg.Envelope, msg.Flags, nil
}

func (c *IMAPSMTPClient) connectIMAP(ctx context.Context, user, pass string) (*imapclient.Client, error) {
	if pass == "" {
		return nil, fmt.Errorf("missing mail credentials")
	}
	dialer := &net.Dialer{Timeout: defaultDialTimeout}
	addr := net.JoinHostPort(c.cfg.IMAPHost, strconv.Itoa(c.cfg.IMAPPort))
	tlsConfig := &tls.Config{ServerName: c.cfg.IMAPHost, InsecureSkipVerify: c.cfg.IMAPInsecureSkipVerify}

	var cli *imapclient.Client
	var err error
	if c.cfg.IMAPTLS {
		cli, err = imapclient.DialWithDialerTLS(dialer, addr, tlsConfig)
	} else {
		cli, err = imapclient.DialWithDialer(dialer, addr)
		if err == nil && c.cfg.IMAPStartTLS {
			err = cli.StartTLS(tlsConfig)
		}
	}
	if err != nil {
		return nil, err
	}
	if err := cli.Login(user, pass); err != nil {
		_ = cli.Logout()
		return nil, err
	}
	return cli, nil
}

func (c *IMAPSMTPClient) sendSMTP(ctx context.Context, user, pass, from string, rcpt []string, raw []byte) error {
	return SubmitSMTP(ctx, SMTPSubmissionConfig{
		Host:               c.cfg.SMTPHost,
		Port:               c.cfg.SMTPPort,
		TLS:                c.cfg.SMTPTLS,
		StartTLS:           c.cfg.SMTPStartTLS,
		InsecureSkipVerify: c.cfg.SMTPInsecureSkipVerify,
		Username:           user,
		Password:           pass,
	}, from, rcpt, raw)
}

func buildRFC822(req SendRequest) ([]byte, error) {
	to := normalizedRecipients(req.To)
	cc := normalizedRecipients(req.CC)
	bodyText := strings.TrimSpace(req.Body)
	bodyHTML := strings.TrimSpace(req.BodyHTML)
	if bodyText == "" && bodyHTML != "" {
		bodyText = plainTextFromHTML(bodyHTML)
	}
	if bodyText == "" {
		bodyText = req.Body
	}
	inlineAttachments := make([]SendAttachment, 0, len(req.Attachments))
	regularAttachments := make([]SendAttachment, 0, len(req.Attachments))
	for _, item := range req.Attachments {
		if item.Inline && strings.TrimSpace(item.ContentID) != "" {
			inlineAttachments = append(inlineAttachments, item)
		} else {
			regularAttachments = append(regularAttachments, item)
		}
	}

	var buf bytes.Buffer
	mixed := multipart.NewWriter(&buf)
	boundary := mixed.Boundary()
	messageID := strings.TrimSpace(req.MessageID)
	headerFromEmail, err := NormalizeMailboxAddress(firstNonEmptyTrimmed(req.HeaderFromEmail, req.From))
	if err != nil {
		return nil, fmt.Errorf("invalid from email address")
	}
	if messageID == "" {
		messageID = generateMessageID(headerFromEmail)
	}

	fmt.Fprintf(&buf, "From: %s\r\n", encodeHeaderAddress(req.HeaderFromName, headerFromEmail))
	if replyTo := strings.TrimSpace(req.ReplyTo); replyTo != "" {
		replyTo, err = NormalizeMailboxAddress(replyTo)
		if err != nil {
			return nil, fmt.Errorf("invalid reply-to address")
		}
		fmt.Fprintf(&buf, "Reply-To: %s\r\n", encodeHeaderAddress("", replyTo))
	}
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(to, ", "))
	if len(cc) > 0 {
		fmt.Fprintf(&buf, "Cc: %s\r\n", strings.Join(cc, ", "))
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", req.Subject)
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "Message-ID: %s\r\n", formatMessageIDHeader(messageID))
	if req.InReplyToID != "" {
		fmt.Fprintf(&buf, "In-Reply-To: %s\r\n", formatMessageIDHeader(req.InReplyToID))
	}
	if refs := formatReferencesHeader(req.References, req.InReplyToID); refs != "" {
		fmt.Fprintf(&buf, "References: %s\r\n", refs)
	}
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=%q\r\n", boundary)
	fmt.Fprintf(&buf, "\r\n")

	if bodyHTML != "" {
		altBuf := bytes.Buffer{}
		alt := multipart.NewWriter(&altBuf)
		plainPart, err := alt.CreatePart(textPartHeader("text/plain; charset=utf-8"))
		if err != nil {
			return nil, err
		}
		if err := writeQuotedPrintablePart(plainPart, bodyText); err != nil {
			return nil, err
		}
		htmlPart, err := alt.CreatePart(textPartHeader("text/html; charset=utf-8"))
		if err != nil {
			return nil, err
		}
		if err := writeQuotedPrintablePart(htmlPart, bodyHTML); err != nil {
			return nil, err
		}
		if err := alt.Close(); err != nil {
			return nil, err
		}

		relatedBuf := bytes.Buffer{}
		related := multipart.NewWriter(&relatedBuf)
		altHeader := make(textproto.MIMEHeader)
		altHeader.Set("Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", alt.Boundary()))
		altPart, err := related.CreatePart(altHeader)
		if err != nil {
			return nil, err
		}
		if _, err := altPart.Write(altBuf.Bytes()); err != nil {
			return nil, err
		}
		for _, a := range inlineAttachments {
			if err := writeAttachmentPart(related, a, true); err != nil {
				return nil, err
			}
		}
		if err := related.Close(); err != nil {
			return nil, err
		}

		relatedHeader := make(textproto.MIMEHeader)
		relatedHeader.Set("Content-Type", fmt.Sprintf("multipart/related; boundary=%q", related.Boundary()))
		relatedPart, err := mixed.CreatePart(relatedHeader)
		if err != nil {
			return nil, err
		}
		if _, err := relatedPart.Write(relatedBuf.Bytes()); err != nil {
			return nil, err
		}
	} else {
		bodyHeader := textPartHeader("text/plain; charset=utf-8")
		p, err := mixed.CreatePart(bodyHeader)
		if err != nil {
			return nil, err
		}
		if err := writeQuotedPrintablePart(p, bodyText); err != nil {
			return nil, err
		}
		for _, a := range inlineAttachments {
			if err := writeAttachmentPart(mixed, a, false); err != nil {
				return nil, err
			}
		}
	}

	for _, a := range regularAttachments {
		if err := writeAttachmentPart(mixed, a, false); err != nil {
			return nil, err
		}
	}

	if err := mixed.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func textPartHeader(contentType string) textproto.MIMEHeader {
	out := make(textproto.MIMEHeader)
	out.Set("Content-Type", contentType)
	out.Set("Content-Transfer-Encoding", "quoted-printable")
	return out
}

func writeQuotedPrintablePart(part io.Writer, body string) error {
	qp := quotedprintable.NewWriter(part)
	if _, err := qp.Write([]byte(body)); err != nil {
		return err
	}
	return qp.Close()
}

func writeAttachmentPart(writer *multipart.Writer, a SendAttachment, treatInline bool) error {
	h := make(textproto.MIMEHeader)
	ct := strings.TrimSpace(a.ContentType)
	if ct == "" {
		ct = "application/octet-stream"
	}
	filename := strings.TrimSpace(a.Filename)
	if filename == "" {
		if treatInline {
			filename = "inline.bin"
		} else {
			filename = "attachment.bin"
		}
	}
	h.Set("Content-Type", fmt.Sprintf("%s; name=%q", ct, filename))
	disposition := "attachment"
	if treatInline && strings.TrimSpace(a.ContentID) != "" {
		disposition = "inline"
		h.Set("Content-ID", fmt.Sprintf("<%s>", normalizeContentID(a.ContentID)))
	}
	h.Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, filename))
	h.Set("Content-Transfer-Encoding", "base64")
	part, err := writer.CreatePart(h)
	if err != nil {
		return err
	}
	enc := base64NewLineEncoder(part)
	if _, err := enc.Write(a.Data); err != nil {
		return err
	}
	return enc.Close()
}

func encodeHeaderAddress(name, email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	if strings.TrimSpace(name) == "" {
		return email
	}
	addr := &stdmail.Address{
		Name:    strings.TrimSpace(name),
		Address: email,
	}
	return addr.String()
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeContentID(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "<")
	v = strings.TrimSuffix(v, ">")
	if v == "" {
		return "inline"
	}
	return v
}

func formatMessageIDHeader(value string) string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return ""
	}
	clean = strings.TrimPrefix(clean, "<")
	clean = strings.TrimSuffix(clean, ">")
	return fmt.Sprintf("<%s>", clean)
}

func formatReferencesHeader(references []string, inReplyTo string) string {
	items := make([]string, 0, len(references)+1)
	seen := map[string]struct{}{}
	appendID := func(value string) {
		clean := strings.TrimSpace(value)
		clean = strings.TrimPrefix(clean, "<")
		clean = strings.TrimSuffix(clean, ">")
		if clean == "" {
			return
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		items = append(items, formatMessageIDHeader(clean))
	}
	for _, ref := range references {
		appendID(ref)
	}
	appendID(inReplyTo)
	return strings.Join(items, " ")
}

func generateMessageID(from string) string {
	domain := "localhost"
	addr := strings.TrimSpace(from)
	if idx := strings.LastIndex(addr, "@"); idx >= 0 && idx+1 < len(addr) {
		host := strings.TrimSpace(addr[idx+1:])
		host = strings.TrimSuffix(host, ">")
		host = strings.TrimSpace(host)
		if host != "" {
			domain = host
		}
	}
	var token [12]byte
	if _, err := rand.Read(token[:]); err != nil {
		return fmt.Sprintf("%d@%s", time.Now().UTC().UnixNano(), domain)
	}
	return fmt.Sprintf("%s@%s", hex.EncodeToString(token[:]), domain)
}

func mergeRecipients(to, cc, bcc []string) []string {
	out := make([]string, 0, len(to)+len(cc)+len(bcc))
	seen := make(map[string]struct{}, len(out))
	appendUnique := func(items []string) {
		for _, item := range items {
			value := strings.TrimSpace(item)
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, value)
		}
	}
	appendUnique(to)
	appendUnique(cc)
	appendUnique(bcc)
	return out
}

func normalizedRecipients(in []string) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

var (
	htmlNoisePattern      = regexp.MustCompile(`(?is)<(?:style|script|head|svg|noscript)[^>]*>.*?</(?:style|script|head|svg|noscript)>`)
	htmlTagPattern        = regexp.MustCompile(`(?s)<[^>]+>`)
	htmlBreakTagPattern   = regexp.MustCompile(`(?i)<\s*br\s*/?\s*>`)
	htmlCloseBlockPattern = regexp.MustCompile(`(?i)</\s*(p|div|li|h[1-6]|tr|table|blockquote)\s*>`)
	htmlOpenLiPattern     = regexp.MustCompile(`(?i)<\s*li[^>]*>`)
)

func plainTextFromHTML(rawHTML string) string {
	s := strings.ReplaceAll(rawHTML, "\r\n", "\n")
	s = htmlNoisePattern.ReplaceAllString(s, " ")
	s = htmlBreakTagPattern.ReplaceAllString(s, "\n")
	s = htmlCloseBlockPattern.ReplaceAllString(s, "\n")
	s = htmlOpenLiPattern.ReplaceAllString(s, "- ")
	s = htmlTagPattern.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	lines := strings.Split(s, "\n")
	normalized := make([]string, 0, len(lines))
	blankCount := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blankCount++
			if blankCount > 1 {
				continue
			}
			normalized = append(normalized, "")
			continue
		}
		blankCount = 0
		normalized = append(normalized, line)
	}
	return strings.TrimSpace(strings.Join(normalized, "\n"))
}

func PlainTextFromHTML(rawHTML string) string {
	return plainTextFromHTML(rawHTML)
}

type base64LineEncoder struct {
	w io.Writer
	b []byte
}

func base64NewLineEncoder(w io.Writer) *base64LineEncoder {
	return &base64LineEncoder{w: w}
}

func (e *base64LineEncoder) Write(p []byte) (int, error) {
	e.b = append(e.b, p...)
	return len(p), nil
}

func (e *base64LineEncoder) Close() error {
	enc := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	for len(e.b) > 0 {
		chunk := e.b
		if len(chunk) > 57 {
			chunk = chunk[:57]
		}
		out := make([]byte, ((len(chunk)+2)/3)*4)
		j := 0
		for i := 0; i < len(chunk); i += 3 {
			var a, b, c byte
			a = chunk[i]
			if i+1 < len(chunk) {
				b = chunk[i+1]
			}
			if i+2 < len(chunk) {
				c = chunk[i+2]
			}
			out[j] = enc[a>>2]
			out[j+1] = enc[((a&0x03)<<4)|(b>>4)]
			if i+1 < len(chunk) {
				out[j+2] = enc[((b&0x0f)<<2)|(c>>6)]
			} else {
				out[j+2] = '='
			}
			if i+2 < len(chunk) {
				out[j+3] = enc[c&0x3f]
			} else {
				out[j+3] = '='
			}
			j += 4
		}
		if _, err := e.w.Write(out); err != nil {
			return err
		}
		if _, err := io.WriteString(e.w, "\r\n"); err != nil {
			return err
		}
		e.b = e.b[len(chunk):]
	}
	return nil
}

func parseMessage(raw []byte, messageID, mailbox string, uid uint32) (Message, error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return Message{}, err
	}

	msg := Message{ID: messageID, Mailbox: mailbox, UID: uid}
	if from, err := mr.Header.AddressList("From"); err == nil && len(from) > 0 {
		msg.From = FormatAddress(from[0])
	}
	if to, err := mr.Header.AddressList("To"); err == nil {
		msg.To = FormatAddressList(to)
	}
	if cc, err := mr.Header.AddressList("Cc"); err == nil {
		msg.CC = FormatAddressList(cc)
	}
	if bcc, err := mr.Header.AddressList("Bcc"); err == nil {
		msg.BCC = FormatAddressList(bcc)
	}
	if subject, err := mr.Header.Subject(); err == nil {
		msg.Subject = DecodeHeaderText(subject)
	}
	if date, err := mr.Header.Date(); err == nil {
		msg.Date = date
	}
	if messageIDHeader, err := mr.Header.MessageID(); err == nil {
		msg.MessageID = strings.TrimSpace(messageIDHeader)
	}
	if rawReferences := mr.Header.Get("References"); strings.TrimSpace(rawReferences) != "" {
		msg.References = ParseMessageIDList(rawReferences)
	}
	if rawInReplyTo := mr.Header.Get("In-Reply-To"); strings.TrimSpace(rawInReplyTo) != "" {
		replyRefs := ParseMessageIDList(rawInReplyTo)
		if len(replyRefs) > 0 {
			msg.InReplyTo = strings.TrimSpace(replyRefs[0])
			msg.References = NormalizeMessageIDHeaders(append(msg.References, replyRefs...))
		}
	}

	partIdx := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		desc := classifyMIMEPart(part.Header)
		switch desc.kind {
		case mimePartTextPlain:
			body, _ := io.ReadAll(io.LimitReader(part.Body, maxBodyBytes))
			if msg.Body == "" {
				msg.Body = string(body)
			}
		case mimePartTextHTML:
			body, _ := io.ReadAll(io.LimitReader(part.Body, maxBodyBytes))
			if msg.BodyHTML == "" {
				msg.BodyHTML = string(body)
			}
		case mimePartAttachment:
			partIdx++
			size, _ := io.Copy(io.Discard, io.LimitReader(part.Body, maxAttachmentBytes+1))
			if size > maxAttachmentBytes {
				size = maxAttachmentBytes
			}
			filename := desc.filename
			if filename == "" {
				filename = fallbackAttachmentFilename(desc.contentType, partIdx)
			}
			meta := AttachmentMeta{
				ID:          EncodeAttachmentID(messageID, partIdx),
				Filename:    filename,
				ContentType: desc.contentType,
				Size:        size,
				Inline:      desc.inline,
				ContentID:   desc.contentID,
			}
			msg.Attachments = append(msg.Attachments, meta)
		}
	}
	if msg.Body == "" && msg.BodyHTML != "" {
		msg.Body = plainTextFromHTML(msg.BodyHTML)
	}
	PopulateLiveMessageThreadID(&msg)

	return msg, nil
}

func extractAttachment(raw []byte, messageID string, targetPart int) (AttachmentMeta, []byte, error) {
	if targetPart <= 0 {
		return AttachmentMeta{}, nil, fs.ErrNotExist
	}
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return AttachmentMeta{}, nil, err
	}
	partIdx := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		desc := classifyMIMEPart(part.Header)
		if desc.kind != mimePartAttachment {
			continue
		}
		partIdx++
		if partIdx != targetPart {
			continue
		}
		filename := desc.filename
		if filename == "" {
			filename = fallbackAttachmentFilename(desc.contentType, targetPart)
		}
		data, err := io.ReadAll(io.LimitReader(part.Body, maxAttachmentBytes))
		if err != nil {
			return AttachmentMeta{}, nil, err
		}
		meta := AttachmentMeta{
			ID:          EncodeAttachmentID(messageID, targetPart),
			Filename:    filename,
			ContentType: desc.contentType,
			Size:        int64(len(data)),
			Inline:      desc.inline,
			ContentID:   desc.contentID,
		}
		return meta, data, nil
	}
	return AttachmentMeta{}, nil, fmt.Errorf("attachment not found")
}

func envelopeFirstAddress(addrs []*imap.Address) string {
	if len(addrs) == 0 || addrs[0] == nil {
		return ""
	}
	return FormatDisplayAddress(addrs[0].PersonalName, addrs[0].Address())
}

func buildMessageSummary(msg *imap.Message, mailbox, from, subject string, date time.Time, previewSection, threadHeaderSection *imap.BodySectionName) MessageSummary {
	if msg == nil {
		return MessageSummary{
			Mailbox:  mailbox,
			From:     from,
			Subject:  subject,
			Date:     date,
			ThreadID: DeriveLiveThreadID(mailbox, "", "", nil, subject, from),
		}
	}
	preview := ""
	if previewSection != nil {
		body := msg.GetBody(previewSection)
		if body != nil {
			if raw, err := io.ReadAll(io.LimitReader(body, int64(previewSampleBytes*2))); err == nil {
				preview = BuildPreviewFromMIMERawSample(raw, DefaultPreviewMaxChars)
			}
		}
	}
	threadID := deriveLiveThreadID(msg, mailbox, subject, from, threadHeaderSection)
	return MessageSummary{
		ID:       EncodeMessageID(mailbox, msg.Uid),
		Mailbox:  mailbox,
		From:     from,
		Subject:  subject,
		Date:     date,
		Seen:     hasFlag(msg.Flags, imap.SeenFlag),
		Flagged:  hasFlag(msg.Flags, imap.FlaggedFlag),
		Answered: hasFlag(msg.Flags, imap.AnsweredFlag),
		Preview:  preview,
		ThreadID: threadID,
	}
}

func deriveLiveThreadID(msg *imap.Message, mailbox, subject, from string, threadHeaderSection *imap.BodySectionName) string {
	if msg == nil {
		return DeriveLiveThreadID(mailbox, "", "", nil, subject, from)
	}
	messageID := ""
	inReplyTo := ""
	if msg.Envelope != nil {
		messageID = msg.Envelope.MessageId
		inReplyTo = msg.Envelope.InReplyTo
	}
	references := liveThreadReferences(msg, threadHeaderSection)
	return DeriveLiveThreadID(mailbox, messageID, inReplyTo, references, subject, from)
}

func liveThreadReferences(msg *imap.Message, threadHeaderSection *imap.BodySectionName) []string {
	if msg == nil || threadHeaderSection == nil {
		return nil
	}
	body := msg.GetBody(threadHeaderSection)
	if body == nil {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(body, 4096))
	if err != nil || len(raw) == 0 {
		return nil
	}
	header, err := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw))).ReadMIMEHeader()
	if err != nil {
		return nil
	}
	return ParseMessageIDList(header.Get("References"))
}

func hasFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

func canonicalFlag(v string) string {
	s := strings.TrimSpace(strings.ToLower(v))
	switch s {
	case "seen", "\\seen":
		return imap.SeenFlag
	case "answered", "\\answered":
		return imap.AnsweredFlag
	case "flagged", "\\flagged":
		return imap.FlaggedFlag
	case "deleted", "\\deleted":
		return imap.DeletedFlag
	case "draft", "\\draft":
		return imap.DraftFlag
	default:
		if strings.HasPrefix(v, "\\") {
			return v
		}
		if s == "" {
			return imap.SeenFlag
		}
		return "\\" + strings.ToUpper(s[:1]) + s[1:]
	}
}
