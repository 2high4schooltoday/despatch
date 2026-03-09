package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/models"
)

func newV2ScopedStore(t *testing.T) *Store {
	t.Helper()
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
	} {
		if err := db.ApplyMigrationFile(sqdb, migration); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}
	return New(sqdb)
}

func createMailAccountForTest(t *testing.T, st *Store, userID, accountID, login string) {
	t.Helper()
	ctx := context.Background()
	if _, err := st.CreateMailAccount(ctx, models.MailAccount{
		ID:           accountID,
		UserID:       userID,
		DisplayName:  login,
		Login:        login,
		SecretEnc:    "enc",
		IMAPHost:     "127.0.0.1",
		IMAPPort:     993,
		IMAPTLS:      true,
		SMTPHost:     "127.0.0.1",
		SMTPPort:     587,
		SMTPStartTLS: true,
		Status:       "active",
	}); err != nil {
		t.Fatalf("create mail account %s: %v", accountID, err)
	}
}

func TestEnsureScopedIndexedIDsRewritesReferences(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "legacy@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createMailAccountForTest(t, st, u.ID, "acct-legacy", "legacy-login")

	now := time.Now().UTC()
	legacyMessageID := mail.EncodeMessageID("INBOX", 42)
	legacyThreadID := "legacy-thread"

	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO thread_index(
			id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		legacyThreadID, "acct-legacy", "INBOX", "subject", "[]", 1, 0, 1, 0, 0, legacyMessageID, now, now,
	); err != nil {
		t.Fatalf("insert legacy thread: %v", err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO message_index(
			id,account_id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,
			from_value,to_value,cc_value,bcc_value,subject,snippet,body_text,body_html_sanitized,raw_source,
			seen,flagged,answered,draft,has_attachments,importance,dkim_status,spf_status,dmarc_status,phishing_score,
			remote_images_blocked,remote_images_allowed,date_header,internal_date,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		legacyMessageID, "acct-legacy", "INBOX", 42, legacyThreadID, "", "", "",
		"from@example.com", "to@example.com", "", "", "subject", "snippet", "body", "", "raw",
		0, 0, 0, 0, 1, 0, "unknown", "unknown", "unknown", 0.0,
		1, 0, now, now, now, now,
	); err != nil {
		t.Fatalf("insert legacy message: %v", err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO attachment_index(id,message_id,account_id,filename,content_type,size_bytes,inline_part,created_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		"att-1", legacyMessageID, "acct-legacy", "file.txt", "text/plain", 12, 0, now,
	); err != nil {
		t.Fatalf("insert legacy attachment: %v", err)
	}

	if err := st.EnsureScopedIndexedIDs(ctx); err != nil {
		t.Fatalf("EnsureScopedIndexedIDs: %v", err)
	}
	if err := st.EnsureScopedIndexedIDs(ctx); err != nil {
		t.Fatalf("EnsureScopedIndexedIDs second run: %v", err)
	}

	scopedMessageID := mail.NormalizeIndexedMessageID("acct-legacy", legacyMessageID)
	scopedThreadID := mail.NormalizeIndexedThreadID("acct-legacy", legacyThreadID)

	msg, err := st.GetIndexedMessageByID(ctx, "acct-legacy", legacyMessageID)
	if err != nil {
		t.Fatalf("GetIndexedMessageByID legacy lookup: %v", err)
	}
	if msg.ID != scopedMessageID || msg.ThreadID != scopedThreadID {
		t.Fatalf("unexpected scoped message/thread ids: %q %q", msg.ID, msg.ThreadID)
	}

	attachments, err := st.GetIndexedMessageAttachments(ctx, "acct-legacy", legacyMessageID)
	if err != nil {
		t.Fatalf("GetIndexedMessageAttachments: %v", err)
	}
	if len(attachments) != 1 || attachments[0].MessageID != scopedMessageID {
		t.Fatalf("expected rewritten attachment message id, got %#v", attachments)
	}

	rows, err := st.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check query: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatalf("expected no foreign key violations after migration")
	}
}

func TestUpsertIndexedMessageScopesIDsPerAccount(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	userA, err := st.CreateUser(ctx, "a@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user A: %v", err)
	}
	userB, err := st.CreateUser(ctx, "b@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user B: %v", err)
	}
	createMailAccountForTest(t, st, userA.ID, "acct-a", "a-login")
	createMailAccountForTest(t, st, userB.ID, "acct-b", "b-login")

	legacyMessageID := mail.EncodeMessageID("INBOX", 77)
	legacyThreadID := "thread-77"
	now := time.Now().UTC()
	threadIDA := mail.NormalizeIndexedThreadID("acct-a", legacyThreadID)
	threadIDB := mail.NormalizeIndexedThreadID("acct-b", legacyThreadID)
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO thread_index(
			id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		threadIDA, "acct-a", "INBOX", "subject A", "[]", 0, 0, 0, 0, 0, "", now, now,
	); err != nil {
		t.Fatalf("insert thread A: %v", err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO thread_index(
			id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		threadIDB, "acct-b", "INBOX", "subject B", "[]", 0, 0, 0, 0, 0, "", now, now,
	); err != nil {
		t.Fatalf("insert thread B: %v", err)
	}

	if _, err := st.UpsertIndexedMessage(ctx, models.IndexedMessage{
		ID:           legacyMessageID,
		AccountID:    "acct-a",
		Mailbox:      "INBOX",
		UID:          77,
		ThreadID:     legacyThreadID,
		Subject:      "subject A",
		FromValue:    "a@example.com",
		ToValue:      "dest@example.com",
		Snippet:      "a",
		BodyText:     "a",
		DateHeader:   now,
		InternalDate: now,
	}); err != nil {
		t.Fatalf("upsert message A: %v", err)
	}
	if _, err := st.UpsertIndexedMessage(ctx, models.IndexedMessage{
		ID:           legacyMessageID,
		AccountID:    "acct-b",
		Mailbox:      "INBOX",
		UID:          77,
		ThreadID:     legacyThreadID,
		Subject:      "subject B",
		FromValue:    "b@example.com",
		ToValue:      "dest@example.com",
		Snippet:      "b",
		BodyText:     "b",
		DateHeader:   now,
		InternalDate: now,
	}); err != nil {
		t.Fatalf("upsert message B: %v", err)
	}

	msgA, err := st.GetIndexedMessageByID(ctx, "acct-a", legacyMessageID)
	if err != nil {
		t.Fatalf("get message A: %v", err)
	}
	msgB, err := st.GetIndexedMessageByID(ctx, "acct-b", legacyMessageID)
	if err != nil {
		t.Fatalf("get message B: %v", err)
	}
	if msgA.Subject == msgB.Subject {
		t.Fatalf("expected per-account rows, got same subject %q", msgA.Subject)
	}

	if err := st.ReplaceIndexedAttachments(ctx, "acct-a", legacyMessageID, []models.IndexedAttachment{
		{ID: "att-a", Filename: "a.txt", ContentType: "text/plain", SizeBytes: 1, CreatedAt: now},
	}); err != nil {
		t.Fatalf("replace attachments A: %v", err)
	}
	if err := st.ReplaceIndexedAttachments(ctx, "acct-b", legacyMessageID, []models.IndexedAttachment{
		{ID: "att-b", Filename: "b.txt", ContentType: "text/plain", SizeBytes: 2, CreatedAt: now},
	}); err != nil {
		t.Fatalf("replace attachments B: %v", err)
	}

	attachmentsA, err := st.GetIndexedMessageAttachments(ctx, "acct-a", legacyMessageID)
	if err != nil {
		t.Fatalf("attachments A: %v", err)
	}
	attachmentsB, err := st.GetIndexedMessageAttachments(ctx, "acct-b", legacyMessageID)
	if err != nil {
		t.Fatalf("attachments B: %v", err)
	}
	if len(attachmentsA) != 1 || len(attachmentsB) != 1 {
		t.Fatalf("unexpected attachment counts: A=%d B=%d", len(attachmentsA), len(attachmentsB))
	}
	if attachmentsA[0].ID == attachmentsB[0].ID {
		t.Fatalf("expected account-scoped attachment isolation, got same attachment id")
	}
}
