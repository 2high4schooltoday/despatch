package workers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap"

	"despatch/internal/config"
	"despatch/internal/mail"
	mailsecclient "despatch/internal/mailsec"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/util"
)

type mailSyncClient interface {
	ListMailboxSnapshots(ctx context.Context, user, pass string) ([]mail.MailboxSnapshot, error)
	ListRecentUIDs(ctx context.Context, user, pass, mailbox string, limit int) ([]uint32, error)
	FetchSyncMessagesByUIDs(ctx context.Context, user, pass, mailbox string, uids []uint32) ([]mail.SyncMessage, error)
}

var newMailSyncClient = func(cfg config.Config) mailSyncClient {
	return mail.NewIMAPSMTPClient(cfg)
}

var newMailClientFactory = func(cfg config.Config) mail.Client {
	return mail.NewIMAPSMTPClient(cfg)
}

var (
	mailSyncLoopInterval  = 10 * time.Second
	mailSyncBaseInterval  = 60 * time.Second
	mailSyncAccountJitter = 15 * time.Second
	mailSyncFailureStep   = 30 * time.Second
	mailSyncMaxBackoff    = 5 * time.Minute
	mailSyncBatchSize     = 100
	mailSyncFullScanLimit = 200
)

const settingThreadSyncRepairDonePrefix = "mail.thread.sync_repair_v1."

type accountSyncSchedule struct {
	nextRunAt time.Time
	failures  int
}

type syncAccountResult struct {
	touchedThreadIDs []string
	fullRebuild      bool
}

type MailWorkers struct {
	cfg               config.Config
	baseCtx           context.Context
	st                *store.Store
	encryptKey        []byte
	now               func() time.Time
	syncClientFactory func(config.Config) mailSyncClient
	mailClientFactory func(config.Config) mail.Client
	scheduleMu        sync.Mutex
	schedules         map[string]accountSyncSchedule
	actionMu          sync.Mutex
	activeAccounts    map[string]struct{}
	healthActions     map[string]models.MailHealthActionState
}

func StartMailWorkers(ctx context.Context, cfg config.Config, st *store.Store) *MailWorkers {
	w := &MailWorkers{
		cfg:               cfg,
		baseCtx:           ctx,
		st:                st,
		encryptKey:        util.Derive32ByteKey(cfg.SessionEncryptKey),
		now:               time.Now,
		syncClientFactory: newMailSyncClient,
		mailClientFactory: newMailClientFactory,
		schedules:         map[string]accountSyncSchedule{},
		activeAccounts:    map[string]struct{}{},
		healthActions:     map[string]models.MailHealthActionState{},
	}
	go w.runSyncLoop(ctx)
	go w.runScheduledSendLoop(ctx)
	return w
}

func (w *MailWorkers) runSyncLoop(ctx context.Context) {
	w.syncDueAccounts(ctx)
	ticker := time.NewTicker(mailSyncLoopInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.syncDueAccounts(ctx)
		}
	}
}

func (w *MailWorkers) syncDueAccounts(ctx context.Context) {
	accounts, err := w.st.ListAllMailAccounts(ctx)
	if err != nil {
		log.Printf("mail_sync list_accounts_failed error=%v", err)
		return
	}
	now := w.now().UTC()
	for _, account := range accounts {
		if !w.accountSyncDue(account.ID, now) {
			continue
		}
		if !w.tryBeginAccountAction(account.ID) {
			continue
		}
		accountCtx, cancel := context.WithTimeout(ctx, 55*time.Second)
		err := w.runAccountSync(accountCtx, account)
		cancel()
		w.finishAccountAction(account.ID)
		if err != nil {
			log.Printf("mail_sync account=%s status=error error=%v", account.ID, err)
			continue
		}
	}
}

func (w *MailWorkers) QueueAccountSync(ctx context.Context, account models.MailAccount) (models.MailHealthActionState, error) {
	return w.queueHealthAction(account, "sync", 55*time.Second, func(actionCtx context.Context) error {
		return w.runAccountSync(actionCtx, account)
	})
}

func (w *MailWorkers) QueueQuotaRefresh(ctx context.Context, account models.MailAccount) (models.MailHealthActionState, error) {
	return w.queueHealthAction(account, "quota_refresh", 30*time.Second, func(actionCtx context.Context) error {
		return w.refreshAccountQuota(actionCtx, account)
	})
}

func (w *MailWorkers) QueueAccountReindex(ctx context.Context, account models.MailAccount) (models.MailHealthActionState, error) {
	return w.queueHealthAction(account, "reindex", 2*time.Minute, func(actionCtx context.Context) error {
		return w.reindexAccount(actionCtx, account)
	})
}

func (w *MailWorkers) ActionState(accountID string) (models.MailHealthActionState, bool) {
	w.actionMu.Lock()
	defer w.actionMu.Unlock()
	state, ok := w.healthActions[strings.TrimSpace(accountID)]
	return state, ok
}

func (w *MailWorkers) queueHealthAction(account models.MailAccount, kind string, timeout time.Duration, run func(context.Context) error) (models.MailHealthActionState, error) {
	accountID := strings.TrimSpace(account.ID)
	if accountID == "" {
		return models.MailHealthActionState{}, service.ErrMailHealthUnavailable
	}
	if !w.tryBeginAccountAction(accountID) {
		return models.MailHealthActionState{}, service.ErrMailHealthActionInProgress
	}
	queued := models.MailHealthActionState{
		Kind:      kind,
		Status:    "queued",
		UpdatedAt: w.now().UTC(),
	}
	w.setHealthActionState(accountID, queued)
	go func() {
		running := queued
		running.Status = "running"
		running.UpdatedAt = w.now().UTC()
		w.setHealthActionState(accountID, running)
		ctx := w.baseCtx
		if ctx == nil {
			ctx = context.Background()
		}
		actionCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		err := run(actionCtx)
		if err != nil {
			failed := models.MailHealthActionState{
				Kind:      kind,
				Status:    "failed",
				Error:     err.Error(),
				UpdatedAt: w.now().UTC(),
			}
			w.setHealthActionState(accountID, failed)
			w.finishAccountAction(accountID)
			return
		}
		w.clearHealthActionState(accountID)
		w.finishAccountAction(accountID)
	}()
	return queued, nil
}

func (w *MailWorkers) syncAccount(ctx context.Context, account models.MailAccount) (syncAccountResult, error) {
	if strings.TrimSpace(account.SecretEnc) == "" || strings.TrimSpace(account.Login) == "" {
		return syncAccountResult{}, errors.New("missing account credentials")
	}
	pass, err := util.DecryptString(w.encryptKey, account.SecretEnc)
	if err != nil {
		return syncAccountResult{}, err
	}
	cli := w.syncClientFactory(w.accountMailConfig(account))
	snapshots, err := cli.ListMailboxSnapshots(ctx, account.Login, pass)
	if err != nil {
		return syncAccountResult{}, err
	}
	touchedThreadIDs := map[string]struct{}{}
	fullRebuild := false
	repairNeeded := false
	for _, snapshot := range snapshots {
		name := strings.TrimSpace(snapshot.Mailbox.Name)
		if name == "" {
			continue
		}
		if err := w.syncMailbox(ctx, cli, account, pass, snapshot, touchedThreadIDs, &fullRebuild, &repairNeeded); err != nil {
			return syncAccountResult{}, err
		}
	}

	forceRepair := false
	if !fullRebuild {
		forceRepair, err = w.needsOneTimeThreadSyncRepair(ctx, account.ID)
		if err != nil {
			return syncAccountResult{}, err
		}
	}

	if fullRebuild {
		if err := w.st.RepairIndexedThreadHeadersByAccount(ctx, account.ID); err != nil {
			return syncAccountResult{}, err
		}
		if err := w.st.RebuildThreadIndex(ctx, account.ID); err != nil {
			return syncAccountResult{}, err
		}
		if err := w.markOneTimeThreadSyncRepairDone(ctx, account.ID); err != nil {
			return syncAccountResult{}, err
		}
	} else {
		if forceRepair || repairNeeded {
			repairedThreadIDs, err := w.st.RepairIndexedThreadHeadersByAccountWithTouchedThreads(ctx, account.ID)
			if err != nil {
				return syncAccountResult{}, err
			}
			for _, threadID := range repairedThreadIDs {
				touchedThreadIDs[threadID] = struct{}{}
			}
		}
		threadIDs := make([]string, 0, len(touchedThreadIDs))
		for threadID := range touchedThreadIDs {
			threadIDs = append(threadIDs, threadID)
		}
		if err := w.st.RefreshThreadIndex(ctx, account.ID, threadIDs); err != nil {
			return syncAccountResult{}, err
		}
		if forceRepair {
			if err := w.markOneTimeThreadSyncRepairDone(ctx, account.ID); err != nil {
				return syncAccountResult{}, err
			}
		}
	}
	threadIDs := make([]string, 0, len(touchedThreadIDs))
	for threadID := range touchedThreadIDs {
		threadIDs = append(threadIDs, threadID)
	}
	return syncAccountResult{touchedThreadIDs: threadIDs, fullRebuild: fullRebuild}, nil
}

func threadSyncRepairSettingKey(accountID string) string {
	return settingThreadSyncRepairDonePrefix + strings.TrimSpace(accountID)
}

func (w *MailWorkers) needsOneTimeThreadSyncRepair(ctx context.Context, accountID string) (bool, error) {
	value, ok, err := w.st.GetSetting(ctx, threadSyncRepairSettingKey(accountID))
	if err != nil {
		return false, err
	}
	if ok && strings.EqualFold(strings.TrimSpace(value), "done") {
		return false, nil
	}
	return true, nil
}

func (w *MailWorkers) markOneTimeThreadSyncRepairDone(ctx context.Context, accountID string) error {
	return w.st.UpsertSetting(ctx, threadSyncRepairSettingKey(accountID), "done")
}

func (w *MailWorkers) runAccountSync(ctx context.Context, account models.MailAccount) error {
	now := w.now().UTC()
	_, err := w.syncAccount(ctx, account)
	if err != nil {
		_ = w.st.UpdateMailAccountSyncStatus(ctx, account.ID, now, err.Error())
		w.recordAccountSyncFailure(account.ID, now)
		return err
	}
	_ = w.st.UpdateMailAccountSyncStatus(ctx, account.ID, now, "")
	w.recordAccountSyncSuccess(account.ID, now)
	return nil
}

func (w *MailWorkers) reindexAccount(ctx context.Context, account models.MailAccount) error {
	if strings.TrimSpace(account.SecretEnc) == "" || strings.TrimSpace(account.Login) == "" {
		return errors.New("missing account credentials")
	}
	pass, err := util.DecryptString(w.encryptKey, account.SecretEnc)
	if err != nil {
		return err
	}
	cli := w.syncClientFactory(w.accountMailConfig(account))
	snapshots, err := cli.ListMailboxSnapshots(ctx, account.Login, pass)
	if err != nil {
		now := w.now().UTC()
		_ = w.st.UpdateMailAccountSyncStatus(ctx, account.ID, now, err.Error())
		w.recordAccountSyncFailure(account.ID, now)
		return err
	}
	if err := w.st.DeleteIndexedDataByAccount(ctx, account.ID); err != nil {
		return err
	}
	touchedThreadIDs := map[string]struct{}{}
	fullRebuild := false
	for _, snapshot := range snapshots {
		name := strings.TrimSpace(snapshot.Mailbox.Name)
		if name == "" {
			continue
		}
		if err := w.syncMailbox(ctx, cli, account, pass, snapshot, touchedThreadIDs, &fullRebuild, nil); err != nil {
			now := w.now().UTC()
			_ = w.st.UpdateMailAccountSyncStatus(ctx, account.ID, now, err.Error())
			w.recordAccountSyncFailure(account.ID, now)
			return err
		}
	}
	if err := w.st.RepairIndexedThreadHeadersByAccount(ctx, account.ID); err != nil {
		now := w.now().UTC()
		_ = w.st.UpdateMailAccountSyncStatus(ctx, account.ID, now, err.Error())
		w.recordAccountSyncFailure(account.ID, now)
		return err
	}
	if err := w.st.RebuildThreadIndex(ctx, account.ID); err != nil {
		now := w.now().UTC()
		_ = w.st.UpdateMailAccountSyncStatus(ctx, account.ID, now, err.Error())
		w.recordAccountSyncFailure(account.ID, now)
		return err
	}
	now := w.now().UTC()
	_ = w.st.UpdateMailAccountSyncStatus(ctx, account.ID, now, "")
	w.recordAccountSyncSuccess(account.ID, now)
	return nil
}

func (w *MailWorkers) refreshAccountQuota(ctx context.Context, account models.MailAccount) error {
	if strings.TrimSpace(account.SecretEnc) == "" || strings.TrimSpace(account.Login) == "" {
		return errors.New("missing account credentials")
	}
	pass, err := util.DecryptString(w.encryptKey, account.SecretEnc)
	if err != nil {
		return err
	}
	var cached models.QuotaCache
	cached, err = w.st.GetQuotaCacheByAccount(ctx, account.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	client := w.mailClientFactory(w.accountMailConfig(account))
	reader, ok := client.(mail.QuotaReader)
	if !ok {
		return w.writeQuotaCache(ctx, account.ID, cached, mail.Quota{}, mail.ErrQuotaUnsupported)
	}
	quota, err := reader.GetQuota(ctx, account.Login, pass)
	if err != nil {
		if errors.Is(err, mail.ErrQuotaUnsupported) {
			return w.writeQuotaCache(ctx, account.ID, cached, mail.Quota{}, mail.ErrQuotaUnsupported)
		}
		if writeErr := w.writeQuotaCache(ctx, account.ID, cached, mail.Quota{}, err); writeErr != nil {
			return writeErr
		}
		return err
	}
	return w.writeQuotaCache(ctx, account.ID, cached, quota, nil)
}

func (w *MailWorkers) writeQuotaCache(ctx context.Context, accountID string, cached models.QuotaCache, quota mail.Quota, quotaErr error) error {
	item := cached
	item.AccountID = accountID
	if quotaErr == nil {
		item.UsedBytes = quota.UsedBytes
		item.TotalBytes = quota.TotalBytes
		item.UsedMessages = quota.UsedMessages
		item.TotalMessages = quota.TotalMessages
		item.LastError = ""
	} else if errors.Is(quotaErr, mail.ErrQuotaUnsupported) {
		item.UsedBytes = 0
		item.TotalBytes = 0
		item.UsedMessages = 0
		item.TotalMessages = 0
		item.LastError = mail.ErrQuotaUnsupported.Error()
	} else {
		item.LastError = quotaErr.Error()
	}
	item.RefreshedAt = w.now().UTC()
	_, err := w.st.UpsertQuotaCache(ctx, item)
	if err != nil {
		return err
	}
	if errors.Is(quotaErr, mail.ErrQuotaUnsupported) {
		return nil
	}
	return nil
}

func (w *MailWorkers) syncMailbox(ctx context.Context, cli mailSyncClient, account models.MailAccount, pass string, snapshot mail.MailboxSnapshot, touchedThreadIDs map[string]struct{}, fullRebuild *bool, repairNeeded *bool) error {
	name := strings.TrimSpace(snapshot.Mailbox.Name)
	now := w.now().UTC()
	state, err := w.st.GetSyncState(ctx, account.ID, name)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}

	fullSync := errors.Is(err, store.ErrNotFound) || state.UIDValidity == 0
	resetSync := false
	if !fullSync && snapshot.UIDValidity != 0 && snapshot.UIDValidity != state.UIDValidity {
		fullSync = true
		resetSync = true
	}
	if !fullSync && snapshot.UIDNext < state.UIDNext {
		fullSync = true
		resetSync = true
	}

	syncState := state
	syncState.AccountID = account.ID
	syncState.Mailbox = name
	syncState.UIDValidity = snapshot.UIDValidity
	syncState.UIDNext = snapshot.UIDNext
	syncState.LastError = ""

	if !fullSync && snapshot.UIDNext == state.UIDNext {
		syncState.LastDeltaSyncAt = now
		_, upsertErr := w.st.UpsertSyncState(ctx, syncState)
		return upsertErr
	}

	if resetSync {
		if err := w.st.DeleteIndexedMessagesByMailbox(ctx, account.ID, name); err != nil {
			return err
		}
	}

	var uids []uint32
	if fullSync {
		uids, err = cli.ListRecentUIDs(ctx, account.Login, pass, name, mailSyncFullScanLimit)
		if err != nil {
			return err
		}
		*fullRebuild = true
	} else {
		uids = uidRange(state.UIDNext, snapshot.UIDNext)
	}
	needsRepair, err := w.syncMailboxUIDs(ctx, cli, account, pass, name, uids, touchedThreadIDs)
	if err != nil {
		return err
	}
	if needsRepair && repairNeeded != nil {
		*repairNeeded = true
	}
	if fullSync {
		syncState.LastFullSyncAt = now
	}
	syncState.LastDeltaSyncAt = now
	_, err = w.st.UpsertSyncState(ctx, syncState)
	return err
}

func (w *MailWorkers) syncMailboxUIDs(ctx context.Context, cli mailSyncClient, account models.MailAccount, pass, mailbox string, uids []uint32, touchedThreadIDs map[string]struct{}) (bool, error) {
	orderedUIDs := append([]uint32(nil), uids...)
	sort.Slice(orderedUIDs, func(i, j int) bool { return orderedUIDs[i] < orderedUIDs[j] })
	needsRepair := false
	for start := 0; start < len(orderedUIDs); start += maxInt(mailSyncBatchSize, 1) {
		end := start + maxInt(mailSyncBatchSize, 1)
		if end > len(orderedUIDs) {
			end = len(orderedUIDs)
		}
		items, err := cli.FetchSyncMessagesByUIDs(ctx, account.Login, pass, mailbox, orderedUIDs[start:end])
		if err != nil {
			return false, err
		}
		sort.Slice(items, func(i, j int) bool { return items[i].UID < items[j].UID })
		for _, item := range items {
			itemNeedsRepair, err := w.upsertSyncMessage(ctx, account, mailbox, item, touchedThreadIDs)
			if err != nil {
				log.Printf("mail_sync account=%s mailbox=%s uid=%d message_upsert_failed error=%v", account.ID, mailbox, item.UID, err)
				continue
			}
			if itemNeedsRepair {
				needsRepair = true
			}
		}
	}
	return needsRepair, nil
}

func (w *MailWorkers) upsertSyncMessage(ctx context.Context, account models.MailAccount, mailbox string, item mail.SyncMessage, touchedThreadIDs map[string]struct{}) (bool, error) {
	now := w.now().UTC()
	msg, err := mail.ParseRawMessage(item.Raw, mailbox, item.UID)
	if err != nil {
		return false, err
	}
	messageID := mail.EncodeMessageID(mailbox, item.UID)
	scopedMessageID := mail.NormalizeIndexedMessageID(account.ID, messageID)
	bodyText := strings.TrimSpace(msg.Body)
	bodyHTMLSanitized := ""
	rawSource := string(item.Raw)
	fromValue := strings.TrimSpace(msg.From)
	toValue := strings.Join(msg.To, ", ")
	ccValue := strings.Join(msg.CC, ", ")
	bccValue := strings.Join(msg.BCC, ", ")
	subject := strings.TrimSpace(msg.Subject)
	snippetValue := mail.BestAvailablePreview("", bodyText, bodyHTMLSanitized, rawSource, mail.DefaultPreviewMaxChars)
	dkimStatus := "unknown"
	spfStatus := "unknown"
	dmarcStatus := "unknown"
	phishingScore := 0.0
	remoteImagesBlocked := true
	indexedAttachments := convertMessageAttachments(account.ID, scopedMessageID, msg.Attachments, now)
	hasAttachments := len(indexedAttachments) > 0
	threadID := ""

	if w.cfg.MailSecEnabled {
		analysis, analysisErr := w.parseAndClassifyWithMailSec(ctx, account.ID, scopedMessageID, item.Raw, now)
		if analysisErr != nil {
			log.Printf("mail_sync account=%s message=%s mailsec_parse_failed error=%v", account.ID, messageID, analysisErr)
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
			bodyHTMLSanitized = analysis.BodyHTMLSanitized
			rawSource = analysis.RawSource
			snippetValue = mail.BestAvailablePreview(analysis.Snippet, bodyText, bodyHTMLSanitized, rawSource, mail.DefaultPreviewMaxChars)
			dkimStatus = analysis.DKIMStatus
			spfStatus = analysis.SPFStatus
			dmarcStatus = analysis.DMARCStatus
			phishingScore = analysis.PhishingScore
			remoteImagesBlocked = analysis.RemoteImagesBlocked
			if len(analysis.Attachments) > 0 {
				indexedAttachments = analysis.Attachments
			}
			hasAttachments = hasAttachments || analysis.HasAttachments || len(indexedAttachments) > 0
		}
	}
	fromValue = mail.DecodeAddressListValue(fromValue)
	toValue = mail.DecodeAddressListValue(toValue)
	ccValue = mail.DecodeAddressListValue(ccValue)
	bccValue = mail.DecodeAddressListValue(bccValue)
	subject = mail.DecodeHeaderText(subject)
	threadID = w.resolveIndexedThreadID(ctx, account.ID, msg, subject, fromValue)

	scopedThreadID := mail.NormalizeIndexedThreadID(account.ID, threadID)
	indexed := models.IndexedMessage{
		ID:                  scopedMessageID,
		AccountID:           account.ID,
		Mailbox:             firstNonEmptyString(msg.Mailbox, mailbox),
		UID:                 msg.UID,
		ThreadID:            scopedThreadID,
		MessageIDHeader:     mail.NormalizeMessageIDHeader(msg.MessageID),
		InReplyToHeader:     mail.NormalizeMessageIDHeader(msg.InReplyTo),
		ReferencesHeader:    mail.FormatMessageIDList(msg.References),
		FromValue:           fromValue,
		ToValue:             toValue,
		CCValue:             ccValue,
		BCCValue:            bccValue,
		Subject:             subject,
		Snippet:             snippetValue,
		BodyText:            bodyText,
		BodyHTMLSanitized:   bodyHTMLSanitized,
		RawSource:           rawSource,
		Seen:                hasIMAPFlag(item.Flags, imap.SeenFlag),
		Flagged:             hasIMAPFlag(item.Flags, imap.FlaggedFlag),
		Answered:            hasIMAPFlag(item.Flags, imap.AnsweredFlag),
		Draft:               hasIMAPFlag(item.Flags, imap.DraftFlag),
		HasAttachments:      hasAttachments,
		Importance:          0,
		DKIMStatus:          dkimStatus,
		SPFStatus:           spfStatus,
		DMARCStatus:         dmarcStatus,
		PhishingScore:       phishingScore,
		RemoteImagesBlocked: remoteImagesBlocked,
		RemoteImagesAllowed: false,
		DateHeader:          chooseTime(msg.Date, item.InternalDate, now),
		InternalDate:        chooseTime(item.InternalDate, msg.Date, now),
	}
	if _, err := w.st.UpsertIndexedMessage(ctx, indexed); err != nil {
		return false, err
	}
	if err := w.st.ReplaceIndexedAttachments(ctx, account.ID, scopedMessageID, indexedAttachments); err != nil {
		return false, err
	}
	touchedThreadIDs[scopedThreadID] = struct{}{}
	if referenced, err := w.st.HasIndexedMessagesReferencingThreadParent(ctx, account.ID, indexed.MessageIDHeader, scopedThreadID, scopedMessageID); err == nil && referenced {
		return true, nil
	} else if err != nil {
		return false, err
	}
	return false, nil
}

func (w *MailWorkers) resolveIndexedThreadID(ctx context.Context, accountID string, msg mail.Message, subject, from string) string {
	normalizedRefs := mail.NormalizeMessageIDHeaders(msg.References)
	lookupHeaders := append([]string{}, normalizedRefs...)
	if inReplyTo := mail.NormalizeMessageIDHeader(msg.InReplyTo); inReplyTo != "" {
		lookupHeaders = append(lookupHeaders, inReplyTo)
	}
	if threadID, err := w.st.FindIndexedThreadIDByMessageHeaders(ctx, accountID, lookupHeaders); err == nil && strings.TrimSpace(threadID) != "" {
		return threadID
	}
	return mail.DeriveIndexedThreadID(msg.MessageID, msg.InReplyTo, normalizedRefs, subject, from)
}

func (w *MailWorkers) tryBeginAccountAction(accountID string) bool {
	key := strings.TrimSpace(accountID)
	if key == "" {
		return false
	}
	w.actionMu.Lock()
	defer w.actionMu.Unlock()
	if w.activeAccounts == nil {
		w.activeAccounts = map[string]struct{}{}
	}
	if _, ok := w.activeAccounts[key]; ok {
		return false
	}
	w.activeAccounts[key] = struct{}{}
	return true
}

func (w *MailWorkers) finishAccountAction(accountID string) {
	key := strings.TrimSpace(accountID)
	if key == "" {
		return
	}
	w.actionMu.Lock()
	defer w.actionMu.Unlock()
	delete(w.activeAccounts, key)
}

func (w *MailWorkers) setHealthActionState(accountID string, state models.MailHealthActionState) {
	key := strings.TrimSpace(accountID)
	if key == "" {
		return
	}
	w.actionMu.Lock()
	defer w.actionMu.Unlock()
	if w.healthActions == nil {
		w.healthActions = map[string]models.MailHealthActionState{}
	}
	w.healthActions[key] = state
}

func (w *MailWorkers) clearHealthActionState(accountID string) {
	key := strings.TrimSpace(accountID)
	if key == "" {
		return
	}
	w.actionMu.Lock()
	defer w.actionMu.Unlock()
	delete(w.healthActions, key)
}

func (w *MailWorkers) accountSyncDue(accountID string, now time.Time) bool {
	w.scheduleMu.Lock()
	defer w.scheduleMu.Unlock()
	state, ok := w.schedules[accountID]
	return !ok || state.nextRunAt.IsZero() || !now.Before(state.nextRunAt)
}

func (w *MailWorkers) recordAccountSyncSuccess(accountID string, now time.Time) {
	w.scheduleMu.Lock()
	defer w.scheduleMu.Unlock()
	w.schedules[accountID] = accountSyncSchedule{
		nextRunAt: now.Add(mailSyncBaseInterval + accountSyncJitter(accountID)),
		failures:  0,
	}
}

func (w *MailWorkers) recordAccountSyncFailure(accountID string, now time.Time) {
	w.scheduleMu.Lock()
	defer w.scheduleMu.Unlock()
	state := w.schedules[accountID]
	state.failures++
	state.nextRunAt = now.Add(accountSyncFailureDelay(state.failures) + accountSyncJitter(accountID))
	w.schedules[accountID] = state
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
	queueAccount, err := w.st.GetMailAccountByID(ctx, item.UserID, item.AccountID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = w.st.MarkScheduledSendFailed(ctx, item.ID, "mail account not found")
			_ = w.st.SetDraftSendState(ctx, item.UserID, item.DraftID, "failed", "mail account not found")
			return nil
		}
		return err
	}
	user, err := w.st.GetUserByID(ctx, item.UserID)
	if err != nil {
		return err
	}
	queuePass, err := util.DecryptString(w.encryptKey, queueAccount.SecretEnc)
	if err != nil {
		return err
	}
	buildClient := w.mailClientFactory(w.accountMailConfig(queueAccount))
	svc := service.New(w.cfg, w.st, buildClient, mail.NoopProvisioner{}, nil)
	sendReq, sendAccountID, markAnswered, err := svc.BuildDraftSendRequest(ctx, user, queueAccount.Login, queuePass, draft)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = w.st.MarkScheduledSendFailed(ctx, item.ID, err.Error())
			_ = w.st.SetDraftSendState(ctx, item.UserID, item.DraftID, "failed", err.Error())
			return nil
		}
		return err
	}
	recipients := append([]string{}, sendReq.To...)
	recipients = append(recipients, sendReq.CC...)
	recipients = append(recipients, sendReq.BCC...)
	if len(recipients) == 0 {
		_ = w.st.MarkScheduledSendFailed(ctx, item.ID, "no recipients")
		_ = w.st.SetDraftSendState(ctx, item.UserID, item.DraftID, "failed", "no recipients")
		return nil
	}
	sendAccount := queueAccount
	sendPass := queuePass
	sendClient := buildClient
	if resolvedAccountID := strings.TrimSpace(sendAccountID); resolvedAccountID != "" && resolvedAccountID != queueAccount.ID {
		sendAccount, err = w.st.GetMailAccountByID(ctx, item.UserID, resolvedAccountID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				_ = w.st.MarkScheduledSendFailed(ctx, item.ID, "send account not found")
				_ = w.st.SetDraftSendState(ctx, item.UserID, item.DraftID, "failed", "send account not found")
				return nil
			}
			return err
		}
		sendPass, err = util.DecryptString(w.encryptKey, sendAccount.SecretEnc)
		if err != nil {
			return err
		}
		sendClient = w.mailClientFactory(w.accountMailConfig(sendAccount))
	}
	if _, err := sendClient.Send(ctx, sendAccount.Login, sendPass, sendReq); err != nil {
		retryCount := item.RetryCount + 1
		if retryCount >= 3 {
			_ = w.st.MarkScheduledSendFailed(ctx, item.ID, err.Error())
			_ = w.st.SetDraftSendState(ctx, item.UserID, item.DraftID, "failed", err.Error())
			return nil
		}
		nextRetry := time.Now().UTC().Add(time.Duration(1<<uint(retryCount-1)) * 30 * time.Second)
		_ = w.st.MarkScheduledSendRetry(ctx, item.ID, retryCount, nextRetry, err.Error())
		_ = w.st.SetDraftSendState(ctx, item.UserID, item.DraftID, "retrying", err.Error())
		return nil
	}

	_ = w.st.MarkScheduledSendSent(ctx, item.ID)
	_ = w.st.SetDraftSendState(ctx, item.UserID, item.DraftID, "sent", "")
	if markAnswered && strings.TrimSpace(draft.ContextMessageID) != "" {
		if contextAccountID := strings.TrimSpace(draft.ContextAccountID); contextAccountID != "" {
			if contextAccount, contextErr := w.st.GetMailAccountByID(ctx, item.UserID, contextAccountID); contextErr == nil {
				if contextPass, decryptErr := util.DecryptString(w.encryptKey, contextAccount.SecretEnc); decryptErr == nil {
					contextClient := w.mailClientFactory(w.accountMailConfig(contextAccount))
					_ = contextClient.UpdateFlags(ctx, contextAccount.Login, contextPass, draft.ContextMessageID, mail.FlagPatch{Add: []string{"\\Answered"}})
				}
			}
			_ = w.st.SetIndexedMessageAnswered(ctx, contextAccountID, draft.ContextMessageID, true)
		} else {
			_ = sendClient.UpdateFlags(ctx, sendAccount.Login, sendPass, draft.ContextMessageID, mail.FlagPatch{Add: []string{"\\Answered"}})
		}
	}
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

func hasIMAPFlag(flags []string, target string) bool {
	for _, flag := range flags {
		if strings.EqualFold(strings.TrimSpace(flag), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func uidRange(startInclusive, endExclusive uint32) []uint32 {
	if endExclusive <= startInclusive {
		return []uint32{}
	}
	out := make([]uint32, 0, endExclusive-startInclusive)
	for uid := startInclusive; uid < endExclusive; uid++ {
		out = append(out, uid)
	}
	return out
}

func accountSyncJitter(accountID string) time.Duration {
	if mailSyncAccountJitter <= 0 {
		return 0
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(accountID)))
	max := uint64(mailSyncAccountJitter)
	if max == 0 {
		return 0
	}
	return time.Duration(binary.BigEndian.Uint64(sum[:8]) % max)
}

func accountSyncFailureDelay(failures int) time.Duration {
	if failures <= 0 {
		return mailSyncBaseInterval
	}
	delay := mailSyncBaseInterval
	step := mailSyncFailureStep
	for i := 1; i < failures; i++ {
		if step > mailSyncMaxBackoff-delay {
			return mailSyncMaxBackoff
		}
		delay += step
		if step >= mailSyncMaxBackoff/2 {
			step = mailSyncMaxBackoff
		} else {
			step *= 2
		}
		if delay >= mailSyncMaxBackoff {
			return mailSyncMaxBackoff
		}
	}
	if delay > mailSyncMaxBackoff {
		return mailSyncMaxBackoff
	}
	return delay
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
			InlinePart:  a.Inline,
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
		RawSource:           string(raw),
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
		size, _ := getMapInt64(row, "size_bytes")
		inline, _ := getMapBool(row, "inline")
		out = append(out, models.IndexedAttachment{
			ID:          mail.EncodeAttachmentID(messageID, i+1),
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
