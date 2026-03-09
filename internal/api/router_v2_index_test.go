package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/util"
)

func TestV2ListMessagesUsesIndexedSummaries(t *testing.T) {
	router, st, account := newIndexedRouterWithStore(t)
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "msg-1",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          1,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "thread-1"),
		FromValue:    "Alice <alice@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Status Update",
		Snippet:      "Deployment completed successfully.",
		Seen:         false,
		Flagged:      true,
		Answered:     false,
		DateHeader:   time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "msg-2",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          2,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "thread-2"),
		FromValue:    "Bob <bob@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Roadmap",
		Snippet:      "Q2 planning notes.",
		Seen:         true,
		DateHeader:   time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
	})

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/messages?account_id="+url.QueryEscape(account.ID)+"&mailbox=INBOX&page=1&page_size=10", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []mail.MessageSummary `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Total != 2 || len(payload.Items) != 2 {
		t.Fatalf("expected 2 indexed summaries, got total=%d items=%d", payload.Total, len(payload.Items))
	}
	if payload.Items[0].Preview == "" || payload.Items[0].ThreadID == "" || payload.Items[0].Mailbox != "INBOX" {
		t.Fatalf("expected preview/thread/mailbox in summary, got %+v", payload.Items[0])
	}
}

func TestV2ListMessagesWaitingFiltersLatestExternalThreads(t *testing.T) {
	router, st, account := newIndexedRouterWithStore(t)
	threadA := mail.ScopeIndexedThreadID(account.ID, "thread-a")
	threadB := mail.ScopeIndexedThreadID(account.ID, "thread-b")
	threadC := mail.ScopeIndexedThreadID(account.ID, "thread-c")

	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "wait-old",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          1,
		ThreadID:     threadA,
		FromValue:    "Alice <alice@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Release Plan",
		Snippet:      "First draft",
		Answered:     false,
		DateHeader:   time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "wait-latest",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          2,
		ThreadID:     threadA,
		FromValue:    "Alice <alice@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Re: Release Plan",
		Snippet:      "Following up",
		Answered:     false,
		DateHeader:   time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "self-latest",
		AccountID:    account.ID,
		Mailbox:      "Sent",
		UID:          3,
		ThreadID:     threadB,
		FromValue:    "account@example.com",
		ToValue:      "Bob <bob@example.com>",
		Subject:      "Sent follow-up",
		Snippet:      "Checking in",
		Answered:     false,
		DateHeader:   time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "already-answered",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          4,
		ThreadID:     threadC,
		FromValue:    "Carol <carol@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Status",
		Snippet:      "Did you get this?",
		Answered:     true,
		DateHeader:   time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
	})

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/messages?account_id="+url.QueryEscape(account.ID)+"&view=waiting&page=1&page_size=10", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []mail.MessageSummary `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode waiting payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Total != 1 || len(payload.Items) != 1 {
		t.Fatalf("expected one waiting thread, got total=%d items=%d body=%s", payload.Total, len(payload.Items), rec.Body.String())
	}
	if payload.Items[0].ID != "wait-latest" {
		t.Fatalf("expected latest external message for waiting view, got %+v", payload.Items[0])
	}
}

func TestV2SuggestRecipientsRanksSentRecipientsAndDedupes(t *testing.T) {
	router, st, account := newIndexedRouterWithStore(t)
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "sent-1",
		AccountID:    account.ID,
		Mailbox:      "Sent",
		UID:          1,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "sent-thread-1"),
		FromValue:    "account@example.com",
		ToValue:      "Alice Example <alice@example.com>",
		CCValue:      "Bob <bob@example.com>",
		Subject:      "Project",
		Snippet:      "Project update",
		DateHeader:   time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "sent-2",
		AccountID:    account.ID,
		Mailbox:      "Sent",
		UID:          2,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "sent-thread-2"),
		FromValue:    "account@example.com",
		ToValue:      "alice@example.com",
		Subject:      "Follow-up",
		Snippet:      "Checking back",
		DateHeader:   time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "inbound-1",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          3,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "inbound-thread-1"),
		FromValue:    "Carol <carol@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Question",
		Snippet:      "Can you review this?",
		DateHeader:   time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
	})

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/recipients/suggest?account_id="+url.QueryEscape(account.ID)+"&q=&limit=5", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []models.RecipientSuggestion `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode recipient suggestions: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Items) < 3 {
		t.Fatalf("expected at least three recipient suggestions, got %+v", payload.Items)
	}
	if payload.Items[0].Email != "alice@example.com" {
		t.Fatalf("expected most frequent sent recipient first, got %+v", payload.Items)
	}
	for _, item := range payload.Items {
		if item.Email == "account@example.com" {
			t.Fatalf("expected self address to be excluded, got %+v", payload.Items)
		}
	}
}

func newIndexedRouterWithStore(t *testing.T) (http.Handler, *store.Store, models.MailAccount) {
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
	if err := st.EnsureAdmin(ctx, "admin@example.com", pwHash); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}
	admin, err := st.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	if err := st.UpdateUserMailLogin(ctx, admin.ID, "account@example.com"); err != nil {
		t.Fatalf("set mail_login: %v", err)
	}
	secret, err := util.EncryptString(util.Derive32ByteKey(sendTestSessionEncryptKey), "mail-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	account, err := st.CreateMailAccount(ctx, models.MailAccount{
		UserID:       admin.ID,
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

	cfg := config.Config{
		ListenAddr:          ":8080",
		BaseDomain:          "example.com",
		SessionCookieName:   "despatch_session",
		CSRFCookieName:      "despatch_csrf",
		SessionIdleMinutes:  30,
		SessionAbsoluteHour: 24,
		SessionEncryptKey:   sendTestSessionEncryptKey,
		CookieSecureMode:    "never",
		TrustProxy:          false,
		PasswordMinLength:   12,
		PasswordMaxLength:   128,
		DovecotAuthMode:     "sql",
	}
	svc := service.New(cfg, st, mail.NoopClient{}, mail.NoopProvisioner{}, nil)
	return NewRouter(cfg, svc), st, account
}

func seedIndexedTestMessage(t *testing.T, st *store.Store, item models.IndexedMessage) {
	t.Helper()
	item.ID = mail.NormalizeIndexedMessageID(item.AccountID, item.ID)
	item.ThreadID = mail.NormalizeIndexedThreadID(item.AccountID, item.ThreadID)
	if item.MessageIDHeader == "" {
		item.MessageIDHeader = item.ID + "@example.com"
	}
	if item.Subject == "" {
		item.Subject = "(no subject)"
	}
	if item.Snippet == "" {
		item.Snippet = "preview"
	}
	if item.BodyText == "" {
		item.BodyText = item.Snippet
	}
	if item.RawSource == "" {
		item.RawSource = item.BodyText
	}
	if item.DateHeader.IsZero() {
		item.DateHeader = time.Now().UTC()
	}
	if item.InternalDate.IsZero() {
		item.InternalDate = item.DateHeader
	}
	if _, err := st.UpsertIndexedMessage(context.Background(), item); err != nil {
		t.Fatalf("upsert indexed message: %v", err)
	}
}
