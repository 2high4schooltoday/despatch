package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"mailclient/internal/auth"
	"mailclient/internal/config"
	"mailclient/internal/db"
	"mailclient/internal/mail"
	"mailclient/internal/service"
	"mailclient/internal/store"
	"mailclient/internal/util"
)

type sendTestMailClient struct {
	mu           sync.Mutex
	sendErr      error
	capturedReq  mail.SendRequest
	capturedUser string
}

func (m *sendTestMailClient) ListMailboxes(ctx context.Context, user, pass string) ([]mail.Mailbox, error) {
	return []mail.Mailbox{{Name: "INBOX", Messages: 1}}, nil
}

func (m *sendTestMailClient) ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]mail.MessageSummary, error) {
	return nil, nil
}

func (m *sendTestMailClient) GetMessage(ctx context.Context, user, pass, id string) (mail.Message, error) {
	return mail.Message{}, nil
}

func (m *sendTestMailClient) Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]mail.MessageSummary, error) {
	return nil, nil
}

func (m *sendTestMailClient) Send(ctx context.Context, user, pass string, req mail.SendRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capturedUser = user
	m.capturedReq = req
	return m.sendErr
}

func (m *sendTestMailClient) SetFlags(ctx context.Context, user, pass, id string, flags []string) error {
	return nil
}

func (m *sendTestMailClient) Move(ctx context.Context, user, pass, id, mailbox string) error {
	return nil
}

func (m *sendTestMailClient) GetAttachment(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentContent, error) {
	return mail.AttachmentContent{}, nil
}

func (m *sendTestMailClient) GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentMeta, io.ReadCloser, error) {
	return mail.AttachmentMeta{}, nil, errors.New("not implemented")
}

func (m *sendTestMailClient) snapshot() (string, mail.SendRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.capturedUser, m.capturedReq
}

func newSendRouter(t *testing.T, mailClient mail.Client, mailLogin string) http.Handler {
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
	if err := st.EnsureAdmin(context.Background(), "admin@example.com", pwHash); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}
	if mailLogin != "" {
		admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
		if err != nil {
			t.Fatalf("load admin: %v", err)
		}
		if err := st.UpdateUserMailLogin(context.Background(), admin.ID, mailLogin); err != nil {
			t.Fatalf("set mail_login: %v", err)
		}
	}

	cfg := config.Config{
		ListenAddr:          ":8080",
		BaseDomain:          "example.com",
		SessionCookieName:   "mailclient_session",
		CSRFCookieName:      "mailclient_csrf",
		SessionIdleMinutes:  30,
		SessionAbsoluteHour: 24,
		SessionEncryptKey:   "this_is_a_valid_long_session_encrypt_key_123456",
		CookieSecureMode:    "never",
		TrustProxy:          false,
		PasswordMinLength:   12,
		PasswordMaxLength:   128,
		DovecotAuthMode:     "sql",
	}

	svc := service.New(cfg, st, mailClient, mail.NoopProvisioner{}, nil)
	return NewRouter(cfg, svc)
}

func loginForSend(t *testing.T, router http.Handler) (*http.Cookie, *http.Cookie) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    "admin@example.com",
		"password": "SecretPass123!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var sessionCookie *http.Cookie
	var csrfCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "mailclient_session" {
			sessionCookie = c
		}
		if c.Name == "mailclient_csrf" {
			csrfCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatalf("missing session cookie")
	}
	if csrfCookie == nil {
		t.Fatalf("missing csrf cookie")
	}
	return sessionCookie, csrfCookie
}

func postSendJSON(t *testing.T, router http.Handler, sessionCookie, csrfCookie *http.Cookie, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestSendSMTPPolicyErrorMappedTo422(t *testing.T) {
	mailClient := &sendTestMailClient{
		sendErr: mail.WrapSMTPSenderRejected(errors.New("sender address rejected: not owned by user")),
	}
	router := newSendRouter(t, mailClient, "webmaster")
	sessionCookie, csrfCookie := loginForSend(t, router)

	body, _ := json.Marshal(map[string]any{
		"to":      []string{"alice@example.com"},
		"subject": "hello",
		"body":    "world",
	})
	rec := postSendJSON(t, router, sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode api error: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "smtp_sender_rejected" {
		t.Fatalf("expected smtp_sender_rejected, got %q body=%s", apiErr.Code, rec.Body.String())
	}

	user, req := mailClient.snapshot()
	if user != "webmaster" {
		t.Fatalf("expected SMTP auth user to use mail_login, got %q", user)
	}
	if req.From != "admin@example.com" {
		t.Fatalf("expected forced From header admin@example.com, got %q", req.From)
	}
}

func TestSendGenericSMTPErrorMappedTo502(t *testing.T) {
	mailClient := &sendTestMailClient{
		sendErr: errors.New("upstream smtp timeout"),
	}
	router := newSendRouter(t, mailClient, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	body, _ := json.Marshal(map[string]any{
		"to":      []string{"alice@example.com"},
		"subject": "hello",
		"body":    "world",
	})
	rec := postSendJSON(t, router, sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode api error: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "smtp_error" {
		t.Fatalf("expected smtp_error, got %q body=%s", apiErr.Code, rec.Body.String())
	}
}

func TestSendIgnoresClientFromField(t *testing.T) {
	mailClient := &sendTestMailClient{}
	router := newSendRouter(t, mailClient, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	body, _ := json.Marshal(map[string]any{
		"from":    "attacker@example.net",
		"to":      []string{"alice@example.com"},
		"subject": "hello",
		"body":    "world",
	})
	rec := postSendJSON(t, router, sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	user, req := mailClient.snapshot()
	if user != "admin@example.com" {
		t.Fatalf("expected SMTP auth user to default to account email, got %q", user)
	}
	if req.From != "admin@example.com" {
		t.Fatalf("expected forced From header admin@example.com, got %q", req.From)
	}
}
