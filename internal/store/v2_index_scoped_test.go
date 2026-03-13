package store

import (
	"context"
	"errors"
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
		filepath.Join("..", "..", "migrations", "025_session_mail_profiles.sql"),
		filepath.Join("..", "..", "migrations", "026_draft_context_account.sql"),
		filepath.Join("..", "..", "migrations", "027_sender_profiles.sql"),
		filepath.Join("..", "..", "migrations", "028_contacts.sql"),
		filepath.Join("..", "..", "migrations", "029_mail_rules.sql"),
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

func TestEnsureIndexedThreadHeadersRepairedRebuildsHistoricalThreads(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "thread-repair@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createMailAccountForTest(t, st, user.ID, "acct-thread-repair", "thread-repair-login")

	now := time.Now().UTC()
	rootRaw := "From: Alice <alice@example.com>\r\nTo: user@example.com\r\nSubject: Topic\r\nMessage-ID: <root@example.com>\r\n\r\nRoot body"
	replyRaw := "From: Bob <bob@example.com>\r\nTo: user@example.com\r\nSubject: Re: Topic\r\nMessage-ID: <reply@example.com>\r\nIn-Reply-To: <root@example.com>\r\nReferences: <root@example.com>\r\n\r\nReply body"
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO thread_index(
			id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"legacy-thread-a", "acct-thread-repair", "INBOX", "topic", "[]", 1, 0, 0, 0, 0, "msg-root", now, now,
	); err != nil {
		t.Fatalf("insert legacy thread a: %v", err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO thread_index(
			id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"legacy-thread-b", "acct-thread-repair", "INBOX", "topic", "[]", 1, 0, 0, 0, 0, "msg-reply", now.Add(time.Minute), now.Add(time.Minute), now.Add(time.Minute),
	); err != nil {
		t.Fatalf("insert legacy thread b: %v", err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO message_index(
			id,account_id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,
			from_value,to_value,cc_value,bcc_value,subject,snippet,body_text,body_html_sanitized,raw_source,
			seen,flagged,answered,draft,has_attachments,importance,dkim_status,spf_status,dmarc_status,phishing_score,
			remote_images_blocked,remote_images_allowed,date_header,internal_date,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"msg-root", "acct-thread-repair", "INBOX", 1, "legacy-thread-a", "", "", "",
		"Alice <alice@example.com>", "user@example.com", "", "", "Topic", "Root body", "Root body", "", rootRaw,
		0, 0, 0, 0, 0, 0, "unknown", "unknown", "unknown", 0.0,
		1, 0, now, now, now, now,
	); err != nil {
		t.Fatalf("insert root message: %v", err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO message_index(
			id,account_id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,
			from_value,to_value,cc_value,bcc_value,subject,snippet,body_text,body_html_sanitized,raw_source,
			seen,flagged,answered,draft,has_attachments,importance,dkim_status,spf_status,dmarc_status,phishing_score,
			remote_images_blocked,remote_images_allowed,date_header,internal_date,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"msg-reply", "acct-thread-repair", "INBOX", 2, "legacy-thread-b", "", "", "",
		"Bob <bob@example.com>", "user@example.com", "", "", "Re: Topic", "Reply body", "Reply body", "", replyRaw,
		0, 0, 0, 0, 0, 0, "unknown", "unknown", "unknown", 0.0,
		1, 0, now.Add(time.Minute), now.Add(time.Minute), now.Add(time.Minute), now.Add(time.Minute),
	); err != nil {
		t.Fatalf("insert reply message: %v", err)
	}

	if err := st.EnsureIndexedThreadHeadersRepaired(ctx); err != nil {
		t.Fatalf("EnsureIndexedThreadHeadersRepaired: %v", err)
	}
	if err := st.EnsureIndexedThreadHeadersRepaired(ctx); err != nil {
		t.Fatalf("EnsureIndexedThreadHeadersRepaired second run: %v", err)
	}

	root, err := st.GetIndexedMessageByID(ctx, "acct-thread-repair", "msg-root")
	if err != nil {
		t.Fatalf("load repaired root: %v", err)
	}
	reply, err := st.GetIndexedMessageByID(ctx, "acct-thread-repair", "msg-reply")
	if err != nil {
		t.Fatalf("load repaired reply: %v", err)
	}
	if root.ThreadID == "" || reply.ThreadID == "" || root.ThreadID != reply.ThreadID {
		t.Fatalf("expected repaired messages to share one thread, got root=%q reply=%q", root.ThreadID, reply.ThreadID)
	}
	if root.MessageIDHeader != "root@example.com" || reply.InReplyToHeader != "root@example.com" {
		t.Fatalf("expected repaired headers to be backfilled, got root=%+v reply=%+v", root, reply)
	}
	threads, total, err := st.ListThreads(ctx, "acct-thread-repair", "", "", 10, 0)
	if err != nil {
		t.Fatalf("list rebuilt threads: %v", err)
	}
	if total != 1 || len(threads) != 1 || threads[0].MessageCount != 2 {
		t.Fatalf("expected one rebuilt historical thread, got total=%d threads=%+v", total, threads)
	}
}

func TestRepairIndexedThreadHeadersByAccountRecoversFromCorruptedReferences(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "thread-corruption@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createMailAccountForTest(t, st, user.ID, "acct-thread-corruption", "thread-corruption-login")

	now := time.Now().UTC()
	parentThreadID := mail.NormalizeIndexedThreadID("acct-thread-corruption", mail.DeriveIndexedThreadID("<projects-parent@example.com>", "<root@example.com>", nil, "Re: Topic", "alice@example.com"))
	childThreadID := mail.NormalizeIndexedThreadID("acct-thread-corruption", mail.DeriveIndexedThreadID("<sent-child@example.com>", "<projects-parent@example.com>", []string{"-parent@example.com"}, "Re: Topic", "user@example.com"))
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO thread_index(
			id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		parentThreadID, "acct-thread-corruption", "Projects", "topic", "[]", 1, 0, 0, 0, 0, "msg-parent", now, now,
	); err != nil {
		t.Fatalf("insert parent thread: %v", err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO thread_index(
			id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		childThreadID, "acct-thread-corruption", "Sent Messages", "topic", "[]", 1, 0, 0, 0, 0, "msg-child", now.Add(time.Minute), now.Add(time.Minute), now.Add(time.Minute),
	); err != nil {
		t.Fatalf("insert child thread: %v", err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO message_index(
			id,account_id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,
			from_value,to_value,cc_value,bcc_value,subject,snippet,body_text,body_html_sanitized,raw_source,
			seen,flagged,answered,draft,has_attachments,importance,dkim_status,spf_status,dmarc_status,phishing_score,
			remote_images_blocked,remote_images_allowed,date_header,internal_date,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"msg-parent", "acct-thread-corruption", "Projects", 1, parentThreadID, "projects-parent@example.com", "root@example.com", "root@example.com",
		"Alice <alice@example.com>", "user@example.com", "", "", "Re: Topic", "Parent body", "Parent body", "", "",
		0, 0, 0, 0, 0, 0, "unknown", "unknown", "unknown", 0.0,
		1, 0, now, now, now, now,
	); err != nil {
		t.Fatalf("insert parent message: %v", err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO message_index(
			id,account_id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,
			from_value,to_value,cc_value,bcc_value,subject,snippet,body_text,body_html_sanitized,raw_source,
			seen,flagged,answered,draft,has_attachments,importance,dkim_status,spf_status,dmarc_status,phishing_score,
			remote_images_blocked,remote_images_allowed,date_header,internal_date,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"msg-child", "acct-thread-corruption", "Sent Messages", 1, childThreadID, "sent-child@example.com", "projects-parent@example.com", "-parent@example.com",
		"User <user@example.com>", "alice@example.com", "", "", "Re: Topic", "Child body", "Child body", "", "",
		0, 0, 0, 0, 0, 0, "unknown", "unknown", "unknown", 0.0,
		1, 0, now.Add(time.Minute), now.Add(time.Minute), now.Add(time.Minute), now.Add(time.Minute),
	); err != nil {
		t.Fatalf("insert child message: %v", err)
	}

	if err := st.RepairIndexedThreadHeadersByAccount(ctx, "acct-thread-corruption"); err != nil {
		t.Fatalf("RepairIndexedThreadHeadersByAccount: %v", err)
	}

	parent, err := st.GetIndexedMessageByID(ctx, "acct-thread-corruption", "msg-parent")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	child, err := st.GetIndexedMessageByID(ctx, "acct-thread-corruption", "msg-child")
	if err != nil {
		t.Fatalf("load child: %v", err)
	}
	if parent.ThreadID == "" || child.ThreadID == "" || parent.ThreadID != child.ThreadID {
		t.Fatalf("expected repair to merge corrupted references back into one thread, got parent=%q child=%q", parent.ThreadID, child.ThreadID)
	}
}

func TestRepairIndexedThreadHeadersByAccountRepairsBlankThreadIDsFromRawSource(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "thread-blank@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createMailAccountForTest(t, st, user.ID, "acct-thread-blank", "thread-blank-login")

	now := time.Now().UTC()
	rootRaw := "From: Alice <alice@example.com>\r\nTo: user@example.com\r\nSubject: Topic\r\nMessage-ID: <root-blank@example.com>\r\n\r\nRoot body"
	replyRaw := "From: Bob <bob@example.com>\r\nTo: user@example.com\r\nSubject: Re: Topic\r\nMessage-ID: <reply-blank@example.com>\r\nIn-Reply-To: <root-blank@example.com>\r\nReferences: <root-blank@example.com>\r\n\r\nReply body"
	if _, err := st.db.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatalf("disable foreign keys: %v", err)
	}
	defer func() {
		_, _ = st.db.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`)
	}()

	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO message_index(
			id,account_id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,
			from_value,to_value,cc_value,bcc_value,subject,snippet,body_text,body_html_sanitized,raw_source,
			seen,flagged,answered,draft,has_attachments,importance,dkim_status,spf_status,dmarc_status,phishing_score,
			remote_images_blocked,remote_images_allowed,date_header,internal_date,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"msg-root-blank", "acct-thread-blank", "INBOX", 1, "", "", "", "",
		"Alice <alice@example.com>", "user@example.com", "", "", "Topic", "Root body", "Root body", "", rootRaw,
		0, 0, 0, 0, 0, 0, "unknown", "unknown", "unknown", 0.0,
		1, 0, now, now, now, now,
	); err != nil {
		t.Fatalf("insert root blank-thread message: %v", err)
	}
	if _, err := st.db.ExecContext(ctx,
		`INSERT INTO message_index(
			id,account_id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,
			from_value,to_value,cc_value,bcc_value,subject,snippet,body_text,body_html_sanitized,raw_source,
			seen,flagged,answered,draft,has_attachments,importance,dkim_status,spf_status,dmarc_status,phishing_score,
			remote_images_blocked,remote_images_allowed,date_header,internal_date,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"msg-reply-blank", "acct-thread-blank", "INBOX", 2, "", "", "", "",
		"Bob <bob@example.com>", "user@example.com", "", "", "Re: Topic", "Reply body", "Reply body", "", replyRaw,
		0, 0, 0, 0, 0, 0, "unknown", "unknown", "unknown", 0.0,
		1, 0, now.Add(time.Minute), now.Add(time.Minute), now.Add(time.Minute), now.Add(time.Minute),
	); err != nil {
		t.Fatalf("insert reply blank-thread message: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
		t.Fatalf("re-enable foreign keys: %v", err)
	}

	if err := st.RepairIndexedThreadHeadersByAccount(ctx, "acct-thread-blank"); err != nil {
		t.Fatalf("RepairIndexedThreadHeadersByAccount: %v", err)
	}

	root, err := st.GetIndexedMessageByID(ctx, "acct-thread-blank", "msg-root-blank")
	if err != nil {
		t.Fatalf("load repaired root: %v", err)
	}
	reply, err := st.GetIndexedMessageByID(ctx, "acct-thread-blank", "msg-reply-blank")
	if err != nil {
		t.Fatalf("load repaired reply: %v", err)
	}
	if root.ThreadID == "" || reply.ThreadID == "" || root.ThreadID != reply.ThreadID {
		t.Fatalf("expected repair to backfill blank thread ids, got root=%q reply=%q", root.ThreadID, reply.ThreadID)
	}

	threads, total, err := st.ListThreads(ctx, "acct-thread-blank", "INBOX", "", 20, 0)
	if err != nil {
		t.Fatalf("list rebuilt threads: %v", err)
	}
	if total != 1 || len(threads) != 1 {
		t.Fatalf("expected one rebuilt thread, got total=%d threads=%+v", total, threads)
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

func TestListIndexedMessagesByAccountsMergesAndFiltersPerAccount(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "multi@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createMailAccountForTest(t, st, user.ID, "acct-a", "a-login")
	createMailAccountForTest(t, st, user.ID, "acct-b", "b-login")

	seed := func(accountID, messageID, mailbox, subject string, at time.Time) {
		t.Helper()
		if _, err := st.UpsertIndexedMessage(ctx, models.IndexedMessage{
			ID:           messageID,
			AccountID:    accountID,
			Mailbox:      mailbox,
			UID:          uint32(at.Unix() % 1000),
			ThreadID:     "thread-" + messageID,
			FromValue:    accountID + "@example.com",
			ToValue:      "dest@example.com",
			Subject:      subject,
			Snippet:      subject,
			BodyText:     subject,
			DateHeader:   at,
			InternalDate: at,
		}); err != nil {
			t.Fatalf("upsert %s/%s: %v", accountID, messageID, err)
		}
	}

	sharedDate := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	seed("acct-a", "shared", "INBOX", "From A", sharedDate)
	seed("acct-b", "shared", "INBOX", "From B", sharedDate.Add(2*time.Hour))
	seed("acct-a", "custom-a", "Projects/Alpha", "Alpha notes", sharedDate.Add(time.Hour))

	items, total, err := st.ListIndexedMessagesByAccounts(ctx, []string{"acct-a", "acct-b"}, nil, models.IndexedMessageFilter{}, "", 10, 0)
	if err != nil {
		t.Fatalf("ListIndexedMessagesByAccounts: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("expected 3 merged messages, got total=%d len=%d", total, len(items))
	}
	if items[0].AccountID != "acct-b" || mail.UnscopeIndexedMessageID(items[0].ID) != "shared" {
		t.Fatalf("expected latest account B shared message first, got account=%s id=%s", items[0].AccountID, items[0].ID)
	}
	if items[1].AccountID != "acct-a" || items[2].AccountID != "acct-a" {
		t.Fatalf("expected remaining account A messages, got %+v", items)
	}

	filtered, filteredTotal, err := st.ListIndexedMessagesByAccounts(ctx, []string{"acct-a", "acct-b"}, map[string][]string{
		"acct-a": {"Projects/Alpha"},
		"acct-b": {"Projects/Alpha"},
	}, models.IndexedMessageFilter{}, "", 10, 0)
	if err != nil {
		t.Fatalf("ListIndexedMessagesByAccounts filtered: %v", err)
	}
	if filteredTotal != 1 || len(filtered) != 1 {
		t.Fatalf("expected one filtered message, got total=%d len=%d", filteredTotal, len(filtered))
	}
	if filtered[0].Mailbox != "Projects/Alpha" || filtered[0].AccountID != "acct-a" {
		t.Fatalf("expected account A custom mailbox result, got %+v", filtered[0])
	}
}

func TestSearchIndexedMessagesByAccountsMergesAcrossAccounts(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "search@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createMailAccountForTest(t, st, user.ID, "acct-a", "a-login")
	createMailAccountForTest(t, st, user.ID, "acct-b", "b-login")

	for _, item := range []models.IndexedMessage{
		{
			ID:           "alpha-a",
			AccountID:    "acct-a",
			Mailbox:      "INBOX",
			UID:          1,
			ThreadID:     "thread-a",
			FromValue:    "a@example.com",
			ToValue:      "dest@example.com",
			Subject:      "Alpha launch",
			Snippet:      "alpha rollout",
			BodyText:     "alpha rollout",
			DateHeader:   time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
			InternalDate: time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:           "alpha-b",
			AccountID:    "acct-b",
			Mailbox:      "INBOX",
			UID:          1,
			ThreadID:     "thread-b",
			FromValue:    "b@example.com",
			ToValue:      "dest@example.com",
			Subject:      "Alpha follow-up",
			Snippet:      "alpha follow-up",
			BodyText:     "alpha follow-up",
			DateHeader:   time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
			InternalDate: time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		},
	} {
		if _, err := st.UpsertIndexedMessage(ctx, item); err != nil {
			t.Fatalf("upsert indexed message %s: %v", item.ID, err)
		}
	}

	items, total, err := st.SearchIndexedMessagesByAccounts(ctx, []string{"acct-a", "acct-b"}, nil, models.IndexedMessageFilter{
		Query: "alpha",
	}, 10, 0)
	if err != nil {
		t.Fatalf("SearchIndexedMessagesByAccounts: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("expected two merged search results, got total=%d len=%d", total, len(items))
	}
	if items[0].AccountID != "acct-b" || items[1].AccountID != "acct-a" {
		t.Fatalf("expected search results sorted newest first across accounts, got %+v", items)
	}
}

func TestListIndexedMessagesByAccountsAppliesAdvancedFilters(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "filters@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createMailAccountForTest(t, st, user.ID, "acct-a", "a-login")
	createMailAccountForTest(t, st, user.ID, "acct-b", "b-login")

	base := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	for _, item := range []models.IndexedMessage{
		{
			ID:             "match-a",
			AccountID:      "acct-a",
			Mailbox:        "INBOX",
			UID:            1,
			ThreadID:       "thread-match-a",
			FromValue:      "Alice Example <alice@example.com>",
			ToValue:        "team@example.com",
			Subject:        "Project plan",
			Snippet:        "project preview",
			BodyText:       "project preview",
			Seen:           false,
			Flagged:        true,
			HasAttachments: true,
			DateHeader:     base,
			InternalDate:   base,
		},
		{
			ID:           "seen-a",
			AccountID:    "acct-a",
			Mailbox:      "INBOX",
			UID:          2,
			ThreadID:     "thread-seen-a",
			FromValue:    "Alice Example <alice@example.com>",
			ToValue:      "team@example.com",
			Subject:      "Project plan",
			Snippet:      "project preview",
			BodyText:     "project preview",
			Seen:         true,
			Flagged:      true,
			DateHeader:   base,
			InternalDate: base,
		},
		{
			ID:           "other-account",
			AccountID:    "acct-b",
			Mailbox:      "INBOX",
			UID:          1,
			ThreadID:     "thread-other-account",
			FromValue:    "Alice Example <alice@example.com>",
			ToValue:      "team@example.com",
			Subject:      "Project plan",
			Snippet:      "project preview",
			BodyText:     "project preview",
			Seen:         false,
			Flagged:      true,
			DateHeader:   base,
			InternalDate: base,
		},
		{
			ID:           "old-a",
			AccountID:    "acct-a",
			Mailbox:      "INBOX",
			UID:          3,
			ThreadID:     "thread-old-a",
			FromValue:    "Alice Example <alice@example.com>",
			ToValue:      "team@example.com",
			Subject:      "Project plan",
			Snippet:      "project preview",
			BodyText:     "project preview",
			Seen:         false,
			Flagged:      true,
			DateHeader:   base.AddDate(0, 0, -2),
			InternalDate: base.AddDate(0, 0, -2),
		},
	} {
		if _, err := st.UpsertIndexedMessage(ctx, item); err != nil {
			t.Fatalf("upsert indexed message %s: %v", item.ID, err)
		}
	}

	items, total, err := st.ListIndexedMessagesByAccounts(ctx, []string{"acct-a"}, nil, models.IndexedMessageFilter{
		From:           "alice@example.com",
		To:             "team@example.com",
		Subject:        "project",
		Unread:         true,
		Flagged:        true,
		HasAttachments: true,
		DateFrom:       base.Add(-time.Hour),
		HasDateFrom:    true,
		DateTo:         base.Add(time.Hour),
		HasDateTo:      true,
	}, "", 10, 0)
	if err != nil {
		t.Fatalf("ListIndexedMessagesByAccounts filtered: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one filtered result, got total=%d len=%d", total, len(items))
	}
	if mail.UnscopeIndexedMessageID(items[0].ID) != "match-a" {
		t.Fatalf("expected match-a, got %+v", items[0])
	}
}

func TestSearchIndexedMessagesByAccountsAppliesAddressAndSubjectFilters(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "search-filters@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createMailAccountForTest(t, st, user.ID, "acct-a", "a-login")
	createMailAccountForTest(t, st, user.ID, "acct-b", "b-login")

	for _, item := range []models.IndexedMessage{
		{
			ID:           "alpha-a",
			AccountID:    "acct-a",
			Mailbox:      "INBOX",
			UID:          1,
			ThreadID:     "thread-a",
			FromValue:    "Alpha Team <alpha@example.com>",
			ToValue:      "ops@example.com",
			CCValue:      "board@example.com",
			Subject:      "Alpha launch memo",
			Snippet:      "alpha rollout",
			BodyText:     "alpha rollout",
			DateHeader:   time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
			InternalDate: time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		},
		{
			ID:           "alpha-b",
			AccountID:    "acct-b",
			Mailbox:      "INBOX",
			UID:          1,
			ThreadID:     "thread-b",
			FromValue:    "Beta Team <beta@example.com>",
			ToValue:      "ops@example.com",
			Subject:      "Alpha follow-up",
			Snippet:      "alpha follow-up",
			BodyText:     "alpha follow-up",
			DateHeader:   time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
			InternalDate: time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		},
	} {
		if _, err := st.UpsertIndexedMessage(ctx, item); err != nil {
			t.Fatalf("upsert indexed message %s: %v", item.ID, err)
		}
	}

	items, total, err := st.SearchIndexedMessagesByAccounts(ctx, []string{"acct-a", "acct-b"}, nil, models.IndexedMessageFilter{
		Query:   "alpha",
		From:    "alpha@example.com",
		To:      "board@example.com",
		Subject: "launch",
	}, 10, 0)
	if err != nil {
		t.Fatalf("SearchIndexedMessagesByAccounts filtered: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one filtered search result, got total=%d len=%d", total, len(items))
	}
	if items[0].AccountID != "acct-a" || mail.UnscopeIndexedMessageID(items[0].ID) != "alpha-a" {
		t.Fatalf("expected alpha-a from account A, got %+v", items[0])
	}
}

func TestRenameIndexedMailboxUpdatesMessagesAndThreads(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "rename@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createMailAccountForTest(t, st, user.ID, "acct-rename", "rename-login")

	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	if _, err := st.UpsertIndexedMessage(ctx, models.IndexedMessage{
		ID:           "rename-msg",
		AccountID:    "acct-rename",
		Mailbox:      "Projects",
		UID:          7,
		ThreadID:     "rename-thread",
		FromValue:    "alice@example.com",
		ToValue:      "rename@example.com",
		Subject:      "Project update",
		Snippet:      "Project preview",
		BodyText:     "Project preview",
		DateHeader:   now,
		InternalDate: now,
	}); err != nil {
		t.Fatalf("upsert indexed message: %v", err)
	}
	if _, err := st.UpsertSyncState(ctx, SyncState{
		AccountID:   "acct-rename",
		Mailbox:     "Projects",
		UIDValidity: 1,
		UIDNext:     8,
	}); err != nil {
		t.Fatalf("upsert sync state: %v", err)
	}

	threadIDs, err := st.RenameIndexedMailbox(ctx, "acct-rename", "Projects", "Projects/2026")
	if err != nil {
		t.Fatalf("RenameIndexedMailbox: %v", err)
	}
	if err := st.DeleteSyncState(ctx, "acct-rename", "Projects"); err != nil {
		t.Fatalf("DeleteSyncState: %v", err)
	}
	if err := st.RefreshThreadIndex(ctx, "acct-rename", threadIDs); err != nil {
		t.Fatalf("RefreshThreadIndex: %v", err)
	}

	msg, err := st.GetIndexedMessageByID(ctx, "acct-rename", "rename-msg")
	if err != nil {
		t.Fatalf("GetIndexedMessageByID: %v", err)
	}
	if msg.Mailbox != "Projects/2026" {
		t.Fatalf("expected renamed mailbox, got %q", msg.Mailbox)
	}
	threads, total, err := st.ListThreads(ctx, "acct-rename", "", "", 20, 0)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if total != 1 || len(threads) != 1 || threads[0].Mailbox != "Projects/2026" {
		t.Fatalf("expected refreshed thread mailbox, got total=%d threads=%+v", total, threads)
	}
	if _, err := st.GetSyncState(ctx, "acct-rename", "Projects"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected old sync state removed, got err=%v", err)
	}
}

func TestDeleteIndexedMailboxRemovesMessagesAndThreads(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()
	user, err := st.CreateUser(ctx, "delete@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createMailAccountForTest(t, st, user.ID, "acct-delete", "delete-login")

	now := time.Date(2026, 3, 10, 13, 0, 0, 0, time.UTC)
	if _, err := st.UpsertIndexedMessage(ctx, models.IndexedMessage{
		ID:           "delete-msg",
		AccountID:    "acct-delete",
		Mailbox:      "Projects",
		UID:          3,
		ThreadID:     "delete-thread",
		FromValue:    "alice@example.com",
		ToValue:      "delete@example.com",
		Subject:      "Cleanup",
		Snippet:      "cleanup preview",
		BodyText:     "cleanup preview",
		DateHeader:   now,
		InternalDate: now,
	}); err != nil {
		t.Fatalf("upsert indexed message: %v", err)
	}

	threadIDs, err := st.DeleteIndexedMailbox(ctx, "acct-delete", "Projects")
	if err != nil {
		t.Fatalf("DeleteIndexedMailbox: %v", err)
	}
	if err := st.RefreshThreadIndex(ctx, "acct-delete", threadIDs); err != nil {
		t.Fatalf("RefreshThreadIndex: %v", err)
	}

	if _, err := st.GetIndexedMessageByID(ctx, "acct-delete", "delete-msg"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted indexed message, got err=%v", err)
	}
	threads, total, err := st.ListThreads(ctx, "acct-delete", "", "", 20, 0)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if total != 0 || len(threads) != 0 {
		t.Fatalf("expected deleted thread index rows, got total=%d threads=%+v", total, threads)
	}
}
