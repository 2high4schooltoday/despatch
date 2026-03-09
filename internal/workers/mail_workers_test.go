package workers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/models"
	"despatch/internal/store"
	"despatch/internal/util"
)

const workerTestSessionEncryptKey = "this_is_a_valid_long_session_encrypt_key_123456"

type fakeMailSyncClient struct {
	snapshots      []mail.MailboxSnapshot
	snapshotsErr   error
	snapshotCalls  int
	recentUIDs     map[string][]uint32
	recentUIDCalls int
	fetchCalls     int
	messages       map[string]map[uint32]mail.SyncMessage
}

func (f *fakeMailSyncClient) ListMailboxSnapshots(ctx context.Context, user, pass string) ([]mail.MailboxSnapshot, error) {
	f.snapshotCalls++
	if f.snapshotsErr != nil {
		return nil, f.snapshotsErr
	}
	out := make([]mail.MailboxSnapshot, len(f.snapshots))
	copy(out, f.snapshots)
	return out, nil
}

func (f *fakeMailSyncClient) ListRecentUIDs(ctx context.Context, user, pass, mailbox string, limit int) ([]uint32, error) {
	f.recentUIDCalls++
	return append([]uint32(nil), f.recentUIDs[mailbox]...), nil
}

func (f *fakeMailSyncClient) FetchSyncMessagesByUIDs(ctx context.Context, user, pass, mailbox string, uids []uint32) ([]mail.SyncMessage, error) {
	f.fetchCalls++
	out := make([]mail.SyncMessage, 0, len(uids))
	for _, uid := range uids {
		if item, ok := f.messages[mailbox][uid]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func TestMailWorkersSkipUnchangedMailbox(t *testing.T) {
	ctx := context.Background()
	st, _, account := newMailWorkerTestEnv(t)
	_, err := st.UpsertSyncState(ctx, store.SyncState{
		AccountID:   account.ID,
		Mailbox:     "INBOX",
		UIDValidity: 9,
		UIDNext:     12,
		LastError:   "stale",
	})
	if err != nil {
		t.Fatalf("seed sync state: %v", err)
	}

	fake := &fakeMailSyncClient{
		snapshots: []mail.MailboxSnapshot{{
			Mailbox:     mail.Mailbox{Name: "INBOX"},
			UIDValidity: 9,
			UIDNext:     12,
		}},
	}
	worker := newTestMailWorker(st, fake, time.Now)

	result, err := worker.syncAccount(ctx, account)
	if err != nil {
		t.Fatalf("syncAccount: %v", err)
	}
	if result.fullRebuild {
		t.Fatalf("expected unchanged mailbox to skip full rebuild")
	}
	if fake.recentUIDCalls != 0 || fake.fetchCalls != 0 {
		t.Fatalf("expected unchanged mailbox to skip message fetches, got recent=%d fetch=%d", fake.recentUIDCalls, fake.fetchCalls)
	}

	state, err := st.GetSyncState(ctx, account.ID, "INBOX")
	if err != nil {
		t.Fatalf("reload sync state: %v", err)
	}
	if state.LastError != "" {
		t.Fatalf("expected successful skip to clear last error, got %q", state.LastError)
	}
	if state.LastDeltaSyncAt.IsZero() {
		t.Fatalf("expected skip to update last delta sync time")
	}
}

func TestMailWorkersDeltaSyncRefreshesOnlyTouchedThreads(t *testing.T) {
	ctx := context.Background()
	st, sqdb, account := newMailWorkerTestEnv(t)
	keepThreadID := mail.NormalizeIndexedThreadID(account.ID, deriveThreadID("Keep", "Stable Topic", "keep@example.com"))
	seedThreadIndexRow(t, sqdb, account.ID, keepThreadID, "Keep", "seed-keep")
	if _, err := st.UpsertIndexedMessage(ctx, models.IndexedMessage{
		ID:           mail.EncodeMessageID("Keep", 1),
		AccountID:    account.ID,
		Mailbox:      "Keep",
		UID:          1,
		ThreadID:     keepThreadID,
		FromValue:    "keep@example.com",
		ToValue:      "user@example.com",
		Subject:      "Stable Topic",
		Snippet:      "old",
		BodyText:     "old body",
		RawSource:    "old body",
		DateHeader:   time.Now().UTC().Add(-2 * time.Hour),
		InternalDate: time.Now().UTC().Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("seed indexed message: %v", err)
	}
	beforeKeepUpdatedAt := threadUpdatedAt(t, sqdb, account.ID, keepThreadID)

	_, err := st.UpsertSyncState(ctx, store.SyncState{
		AccountID:   account.ID,
		Mailbox:     "INBOX",
		UIDValidity: 1,
		UIDNext:     5,
	})
	if err != nil {
		t.Fatalf("seed sync state: %v", err)
	}

	fake := &fakeMailSyncClient{
		snapshots: []mail.MailboxSnapshot{{
			Mailbox:     mail.Mailbox{Name: "INBOX"},
			UIDValidity: 1,
			UIDNext:     7,
		}},
		messages: map[string]map[uint32]mail.SyncMessage{
			"INBOX": {
				5: syncMessage("INBOX", 5, "Release Plan", "alice@example.com", "delta one"),
				6: syncMessageWithHeaders("INBOX", 6, "Re: Release Plan", "alice@example.com", "delta two", "<release-plan-5@example.com>", []string{"<release-plan-5@example.com>"}),
			},
		},
	}
	worker := newTestMailWorker(st, fake, time.Now)

	result, err := worker.syncAccount(ctx, account)
	if err != nil {
		t.Fatalf("syncAccount: %v", err)
	}
	if result.fullRebuild {
		t.Fatalf("expected delta sync to avoid full rebuild")
	}
	if fake.recentUIDCalls != 0 {
		t.Fatalf("expected delta sync to avoid recent uid scan, got %d", fake.recentUIDCalls)
	}
	if fake.fetchCalls != 1 {
		t.Fatalf("expected one batched fetch for delta sync, got %d", fake.fetchCalls)
	}

	afterKeepUpdatedAt := threadUpdatedAt(t, sqdb, account.ID, keepThreadID)
	if !afterKeepUpdatedAt.Equal(beforeKeepUpdatedAt) {
		t.Fatalf("expected untouched thread row to keep original timestamp, before=%s after=%s", beforeKeepUpdatedAt, afterKeepUpdatedAt)
	}

	items, total, err := st.ListThreads(ctx, account.ID, "", "", 10, 0)
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected untouched and touched threads to both exist, got total=%d items=%d", total, len(items))
	}
}

func TestMailWorkersUIDValidityResetPurgesMailboxAndFullRebuilds(t *testing.T) {
	ctx := context.Background()
	st, sqdb, account := newMailWorkerTestEnv(t)
	staleMessageID := mail.EncodeMessageID("INBOX", 3)
	staleThreadID := mail.NormalizeIndexedThreadID(account.ID, deriveThreadID("INBOX", "Old Topic", "old@example.com"))
	seedThreadIndexRow(t, sqdb, account.ID, staleThreadID, "INBOX", "seed-stale")
	if _, err := st.UpsertIndexedMessage(ctx, models.IndexedMessage{
		ID:           staleMessageID,
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          3,
		ThreadID:     staleThreadID,
		FromValue:    "old@example.com",
		ToValue:      "user@example.com",
		Subject:      "Old Topic",
		Snippet:      "old",
		BodyText:     "old body",
		RawSource:    "old body",
		DateHeader:   time.Now().UTC().Add(-time.Hour),
		InternalDate: time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("seed stale indexed message: %v", err)
	}
	_, err := st.UpsertSyncState(ctx, store.SyncState{
		AccountID:   account.ID,
		Mailbox:     "INBOX",
		UIDValidity: 1,
		UIDNext:     4,
	})
	if err != nil {
		t.Fatalf("seed sync state: %v", err)
	}

	fake := &fakeMailSyncClient{
		snapshots: []mail.MailboxSnapshot{{
			Mailbox:     mail.Mailbox{Name: "INBOX"},
			UIDValidity: 2,
			UIDNext:     2,
		}},
		recentUIDs: map[string][]uint32{
			"INBOX": {1},
		},
		messages: map[string]map[uint32]mail.SyncMessage{
			"INBOX": {
				1: syncMessage("INBOX", 1, "Fresh Topic", "fresh@example.com", "fresh body"),
			},
		},
	}
	worker := newTestMailWorker(st, fake, time.Now)

	result, err := worker.syncAccount(ctx, account)
	if err != nil {
		t.Fatalf("syncAccount: %v", err)
	}
	if !result.fullRebuild {
		t.Fatalf("expected uid validity reset to force full rebuild")
	}
	if fake.recentUIDCalls != 1 {
		t.Fatalf("expected uid validity reset to reload recent messages, got %d calls", fake.recentUIDCalls)
	}
	if _, err := st.GetIndexedMessageByID(ctx, account.ID, staleMessageID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected stale message to be purged on uid validity reset, got err=%v", err)
	}
	if _, err := st.GetIndexedMessageByID(ctx, account.ID, mail.EncodeMessageID("INBOX", 1)); err != nil {
		t.Fatalf("expected fresh message after reset sync: %v", err)
	}

	items, total, err := st.ListThreads(ctx, account.ID, "", "", 10, 0)
	if err != nil {
		t.Fatalf("list threads: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected full rebuild to leave only fresh thread, got total=%d items=%d", total, len(items))
	}
}

func TestMailWorkersFailureBackoffStaggersRetries(t *testing.T) {
	ctx := context.Background()
	st, _, _ := newMailWorkerTestEnv(t)
	fake := &fakeMailSyncClient{snapshotsErr: errors.New("imap unavailable")}
	current := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	worker := newTestMailWorker(st, fake, func() time.Time { return current })

	oldBase := mailSyncBaseInterval
	oldStep := mailSyncFailureStep
	oldMax := mailSyncMaxBackoff
	oldJitter := mailSyncAccountJitter
	mailSyncBaseInterval = 10 * time.Millisecond
	mailSyncFailureStep = 20 * time.Millisecond
	mailSyncMaxBackoff = 50 * time.Millisecond
	mailSyncAccountJitter = 0
	t.Cleanup(func() {
		mailSyncBaseInterval = oldBase
		mailSyncFailureStep = oldStep
		mailSyncMaxBackoff = oldMax
		mailSyncAccountJitter = oldJitter
	})

	worker.syncDueAccounts(ctx)
	if fake.snapshotCalls != 1 {
		t.Fatalf("expected first sync attempt, got %d calls", fake.snapshotCalls)
	}

	current = current.Add(5 * time.Millisecond)
	worker.syncDueAccounts(ctx)
	if fake.snapshotCalls != 1 {
		t.Fatalf("expected retry to be delayed by backoff, got %d calls", fake.snapshotCalls)
	}

	current = current.Add(6 * time.Millisecond)
	worker.syncDueAccounts(ctx)
	if fake.snapshotCalls != 2 {
		t.Fatalf("expected second attempt after first backoff window, got %d calls", fake.snapshotCalls)
	}

	current = current.Add(24 * time.Millisecond)
	worker.syncDueAccounts(ctx)
	if fake.snapshotCalls != 2 {
		t.Fatalf("expected longer backoff after repeated failure, got %d calls", fake.snapshotCalls)
	}

	current = current.Add(6 * time.Millisecond)
	worker.syncDueAccounts(ctx)
	if fake.snapshotCalls != 3 {
		t.Fatalf("expected third attempt after extended backoff, got %d calls", fake.snapshotCalls)
	}
}

func newMailWorkerTestEnv(t *testing.T) (*store.Store, *sql.DB, models.MailAccount) {
	t.Helper()
	ctx := context.Background()
	sqdb, err := db.OpenSQLite(filepath.Join(t.TempDir(), "app.db"), 1, 1, time.Minute)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqdb.Close() })

	for _, migration := range []string{
		filepath.Join("..", "..", "migrations", "001_init.sql"),
		filepath.Join("..", "..", "migrations", "002_users_mail_login.sql"),
		filepath.Join("..", "..", "migrations", "003_cleanup_rejected_users.sql"),
		filepath.Join("..", "..", "migrations", "004_cleanup_rejected_users_casefold.sql"),
		filepath.Join("..", "..", "migrations", "005_admin_query_indexes.sql"),
		filepath.Join("..", "..", "migrations", "006_users_recovery_email.sql"),
		filepath.Join("..", "..", "migrations", "007_mail_accounts.sql"),
		filepath.Join("..", "..", "migrations", "008_mail_index.sql"),
		filepath.Join("..", "..", "migrations", "009_preferences_and_search.sql"),
		filepath.Join("..", "..", "migrations", "010_drafts_schedule.sql"),
		filepath.Join("..", "..", "migrations", "011_rules_sieve.sql"),
		filepath.Join("..", "..", "migrations", "012_mfa_totp_webauthn.sql"),
		filepath.Join("..", "..", "migrations", "013_crypto_keys.sql"),
		filepath.Join("..", "..", "migrations", "014_session_management.sql"),
		filepath.Join("..", "..", "migrations", "015_sync_state.sql"),
		filepath.Join("..", "..", "migrations", "016_quota_and_health.sql"),
		filepath.Join("..", "..", "migrations", "017_mfa_onboarding_flags.sql"),
		filepath.Join("..", "..", "migrations", "018_mfa_usability_trusted_devices.sql"),
		filepath.Join("..", "..", "migrations", "019_users_mail_secret.sql"),
		filepath.Join("..", "..", "migrations", "020_mail_index_scoped_ids.sql"),
		filepath.Join("..", "..", "migrations", "021_password_reset_token_reservations.sql"),
		filepath.Join("..", "..", "migrations", "022_draft_compose_context.sql"),
		filepath.Join("..", "..", "migrations", "023_drafts_nullable_account.sql"),
		filepath.Join("..", "..", "migrations", "024_draft_attachments_and_send_errors.sql"),
		filepath.Join("..", "..", "migrations", "025_session_mail_profiles.sql"),
	} {
		if err := db.ApplyMigrationFile(sqdb, migration); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}

	st := store.New(sqdb)
	pwHash, err := auth.HashPassword("SecretPass123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(ctx, "user@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	secret, err := util.EncryptString(util.Derive32ByteKey(workerTestSessionEncryptKey), "mail-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	account, err := st.CreateMailAccount(ctx, models.MailAccount{
		UserID:       user.ID,
		DisplayName:  "Primary",
		Login:        "account@example.com",
		SecretEnc:    secret,
		IMAPHost:     "imap.example.com",
		IMAPPort:     993,
		IMAPTLS:      true,
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPStartTLS: true,
		IsDefault:    true,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("create mail account: %v", err)
	}
	return st, sqdb, account
}

func newTestMailWorker(st *store.Store, client mailSyncClient, now func() time.Time) *MailWorkers {
	return &MailWorkers{
		cfg:               config.Config{SessionEncryptKey: workerTestSessionEncryptKey},
		st:                st,
		encryptKey:        util.Derive32ByteKey(workerTestSessionEncryptKey),
		now:               now,
		syncClientFactory: func(config.Config) mailSyncClient { return client },
		schedules:         map[string]accountSyncSchedule{},
	}
}

func syncMessage(mailbox string, uid uint32, subject, from, body string) mail.SyncMessage {
	return syncMessageWithHeaders(mailbox, uid, subject, from, body, "", nil)
}

func syncMessageWithHeaders(mailbox string, uid uint32, subject, from, body, inReplyTo string, references []string) mail.SyncMessage {
	replyHeader := ""
	if strings.TrimSpace(inReplyTo) != "" {
		replyHeader = fmt.Sprintf("In-Reply-To: %s\r\n", inReplyTo)
	}
	refsHeader := ""
	if len(references) > 0 {
		refsHeader = fmt.Sprintf("References: %s\r\n", strings.Join(references, " "))
	}
	raw := fmt.Sprintf(
		"From: %s\r\nTo: user@example.com\r\nSubject: %s\r\nDate: Tue, 10 Mar 2026 12:00:00 +0000\r\nMessage-ID: <%s-%d@example.com>\r\n%s%s\r\n%s",
		from,
		subject,
		strings.ToLower(strings.ReplaceAll(subject, " ", "-")),
		uid,
		replyHeader,
		refsHeader,
		body,
	)
	return mail.SyncMessage{
		Mailbox:      mailbox,
		UID:          uid,
		Raw:          []byte(raw),
		Flags:        []string{},
		InternalDate: time.Date(2026, 3, 10, 12, 0, int(uid), 0, time.UTC),
	}
}

func threadUpdatedAt(t *testing.T, sqdb *sql.DB, accountID, threadID string) time.Time {
	t.Helper()
	var updatedAt time.Time
	if err := sqdb.QueryRowContext(context.Background(), `SELECT updated_at FROM thread_index WHERE account_id=? AND id=?`, accountID, threadID).Scan(&updatedAt); err != nil {
		t.Fatalf("load thread updated_at: %v", err)
	}
	return updatedAt
}

func seedThreadIndexRow(t *testing.T, sqdb *sql.DB, accountID, threadID, mailbox, latestMessageID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := sqdb.ExecContext(
		context.Background(),
		`INSERT INTO thread_index(id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		threadID,
		accountID,
		mailbox,
		"seed",
		"[]",
		0,
		0,
		0,
		0,
		0,
		latestMessageID,
		now,
		now,
	); err != nil {
		t.Fatalf("seed thread index row: %v", err)
	}
}
