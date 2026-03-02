package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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

type pamLoginTestMailClient struct {
	acceptPassword string
	acceptedUsers  map[string]bool
	failWith       error
}

func (m pamLoginTestMailClient) ListMailboxes(ctx context.Context, user, pass string) ([]mail.Mailbox, error) {
	if m.failWith != nil {
		return nil, m.failWith
	}
	if pass != m.acceptPassword {
		return nil, errors.New("imap auth failed")
	}
	if len(m.acceptedUsers) > 0 && !m.acceptedUsers[user] {
		return nil, errors.New("imap auth failed")
	}
	return []mail.Mailbox{{Name: "INBOX", Messages: 1}}, nil
}

func (m pamLoginTestMailClient) ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]mail.MessageSummary, error) {
	return nil, nil
}

func (m pamLoginTestMailClient) GetMessage(ctx context.Context, user, pass, id string) (mail.Message, error) {
	return mail.Message{}, nil
}

func (m pamLoginTestMailClient) Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]mail.MessageSummary, error) {
	return nil, nil
}

func (m pamLoginTestMailClient) Send(ctx context.Context, user, pass string, req mail.SendRequest) error {
	return nil
}

func (m pamLoginTestMailClient) SetFlags(ctx context.Context, user, pass, id string, flags []string) error {
	return nil
}

func (m pamLoginTestMailClient) Move(ctx context.Context, user, pass, id, mailbox string) error {
	return nil
}

func (m pamLoginTestMailClient) GetAttachment(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentContent, error) {
	return mail.AttachmentContent{}, nil
}

func (m pamLoginTestMailClient) GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentMeta, io.ReadCloser, error) {
	return mail.AttachmentMeta{}, nil, errors.New("not implemented")
}

func newPAMLoginRouter(t *testing.T, mailClient mail.Client) (http.Handler, *sql.DB) {
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
		DovecotAuthMode:     "pam",
	}

	svc := service.New(cfg, st, mailClient, mail.NoopProvisioner{}, nil)
	return NewRouter(cfg, svc), sqdb
}

func TestLoginPAMVerifierUnavailableReturnsBadGatewayAndDoesNotIncrementRateEvent(t *testing.T) {
	router, sqdb := newPAMLoginRouter(t, pamLoginTestMailClient{
		failWith: errors.New("dial tcp 127.0.0.1:993: connect: connection refused"),
	})

	body, _ := json.Marshal(map[string]string{
		"email":    "admin@example.com",
		"password": "SecretPass123!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected login 502, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode body: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "pam_verifier_unavailable" {
		t.Fatalf("expected pam_verifier_unavailable, got %q body=%s", apiErr.Code, rec.Body.String())
	}

	var count int
	if err := sqdb.QueryRow(`SELECT COUNT(1) FROM rate_limit_events WHERE route='login_failed'`).Scan(&count); err != nil {
		t.Fatalf("count rate events: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no login_failed rate events for verifier outage, got %d", count)
	}
}

func TestLoginInvalidCredentialsIncrementsRateEvent(t *testing.T) {
	router, sqdb := newPAMLoginRouter(t, pamLoginTestMailClient{
		acceptPassword: "SecretPass123!",
		acceptedUsers:  map[string]bool{"admin@example.com": true},
	})

	body, _ := json.Marshal(map[string]string{
		"email":    "admin@example.com",
		"password": "WrongPass123!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected login 401, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode body: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "invalid_credentials" {
		t.Fatalf("expected invalid_credentials, got %q body=%s", apiErr.Code, rec.Body.String())
	}

	var count int
	if err := sqdb.QueryRow(`SELECT COALESCE(SUM(count), 0) FROM rate_limit_events WHERE route='login_failed'`).Scan(&count); err != nil {
		t.Fatalf("sum rate events: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one failed login rate event, got %d", count)
	}
}
