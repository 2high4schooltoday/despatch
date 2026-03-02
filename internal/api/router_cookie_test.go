package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"mailclient/internal/auth"
	"mailclient/internal/config"
	"mailclient/internal/db"
	"mailclient/internal/service"
	"mailclient/internal/store"
)

func newCookieRouter(t *testing.T, cookieMode string, trustProxy bool) http.Handler {
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
		CookieSecureMode:    cookieMode,
		TrustProxy:          trustProxy,
		PasswordMinLength:   12,
		PasswordMaxLength:   128,
		DovecotAuthMode:     "sql",
	}

	svc := service.New(cfg, st, nil, nil, nil)
	return NewRouter(cfg, svc)
}

func loginAndGetSessionCookie(t *testing.T, router http.Handler, forwardedProto string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    "admin@example.com",
		"password": "SecretPass123!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if forwardedProto != "" {
		req.Header.Set("X-Forwarded-Proto", forwardedProto)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "mailclient_session" {
			return c
		}
	}
	t.Fatalf("session cookie missing")
	return nil
}

func TestLoginCookieSecureAutoDirectHTTP(t *testing.T) {
	router := newCookieRouter(t, "auto", false)
	sessionCookie := loginAndGetSessionCookie(t, router, "")
	if sessionCookie.Secure {
		t.Fatalf("expected secure=false for auto mode over direct http")
	}
}

func TestLoginCookieSecureAutoProxyHTTPS(t *testing.T) {
	router := newCookieRouter(t, "auto", true)
	sessionCookie := loginAndGetSessionCookie(t, router, "https")
	if !sessionCookie.Secure {
		t.Fatalf("expected secure=true for auto mode over proxied https")
	}
}

func TestLoginCookieSecureAlwaysAndSessionMe(t *testing.T) {
	router := newCookieRouter(t, "always", false)
	sessionCookie := loginAndGetSessionCookie(t, router, "")
	if !sessionCookie.Secure {
		t.Fatalf("expected secure=true for always mode")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(&http.Cookie{Name: "mailclient_session", Value: sessionCookie.Value})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected /me 200 with valid session cookie, got %d body=%s", rec.Code, rec.Body.String())
	}
}
