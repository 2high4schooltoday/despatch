package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mailclient/internal/auth"
	"mailclient/internal/config"
	"mailclient/internal/db"
	"mailclient/internal/models"
	"mailclient/internal/service"
	"mailclient/internal/store"
	"mailclient/internal/util"
)

type updateTestFixture struct {
	router     http.Handler
	store      *store.Store
	requestDir string
}

func newUpdateFixture(t *testing.T, enabled bool, configured bool) updateTestFixture {
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
	adminHash, err := auth.HashPassword("SecretPass123!")
	if err != nil {
		t.Fatalf("hash admin password: %v", err)
	}
	if err := st.EnsureAdmin(context.Background(), "admin@example.com", adminHash); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}
	userHash, err := auth.HashPassword("UserPass123!")
	if err != nil {
		t.Fatalf("hash user password: %v", err)
	}
	if _, err := st.CreateUser(context.Background(), "user@example.com", userHash, "user", models.UserActive); err != nil {
		t.Fatalf("create user: %v", err)
	}

	base := t.TempDir()
	updateBase := filepath.Join(base, "update")
	unitDir := filepath.Join(base, "units")
	if configured {
		if err := os.MkdirAll(unitDir, 0o755); err != nil {
			t.Fatalf("mkdir unit dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(unitDir, "mailclient-updater.path"), []byte("ok"), 0o644); err != nil {
			t.Fatalf("write updater unit marker: %v", err)
		}
	}

	cfg := config.Config{
		ListenAddr:             ":8080",
		BaseDomain:             "example.com",
		SessionCookieName:      "mailclient_session",
		CSRFCookieName:         "mailclient_csrf",
		SessionIdleMinutes:     30,
		SessionAbsoluteHour:    24,
		SessionEncryptKey:      "this_is_a_valid_long_session_encrypt_key_123456",
		CookieSecureMode:       "never",
		TrustProxy:             false,
		PasswordMinLength:      12,
		PasswordMaxLength:      128,
		DovecotAuthMode:        "sql",
		UpdateEnabled:          enabled,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "new-mail-client",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          updateBase,
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "mailclient",
		UpdateSystemdUnitDir:   unitDir,
	}
	svc := service.New(cfg, st, nil, nil, nil)
	return updateTestFixture{
		router:     NewRouter(cfg, svc),
		store:      st,
		requestDir: filepath.Join(updateBase, "request"),
	}
}

func loginCookies(t *testing.T, router http.Handler, email, password string) (*http.Cookie, *http.Cookie) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
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
	if sessionCookie == nil || csrfCookie == nil {
		t.Fatalf("missing auth cookies")
	}
	return sessionCookie, csrfCookie
}

func TestAdminUpdateStatusRequiresSession(t *testing.T) {
	fx := newUpdateFixture(t, false, false)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/update/status", nil)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUpdateStatusRequiresAdminRole(t *testing.T) {
	fx := newUpdateFixture(t, false, false)
	sessionCookie, _ := loginCookies(t, fx.router, "user@example.com", "UserPass123!")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/update/status", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUpdateCheckEnforcesCSRF(t *testing.T) {
	fx := newUpdateFixture(t, false, false)
	sessionCookie, _ := loginCookies(t, fx.router, "admin@example.com", "SecretPass123!")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update/check", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 csrf failure, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUpdateApplyNotConfigured(t *testing.T) {
	fx := newUpdateFixture(t, true, false)
	sessionCookie, csrfCookie := loginCookies(t, fx.router, "admin@example.com", "SecretPass123!")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update/apply", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode error body: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "updater_not_configured" {
		t.Fatalf("expected updater_not_configured, got %q", apiErr.Code)
	}
}

func TestAdminUpdateApplyQueuesAndBlocksConcurrent(t *testing.T) {
	fx := newUpdateFixture(t, true, true)
	sessionCookie, csrfCookie := loginCookies(t, fx.router, "admin@example.com", "SecretPass123!")

	sendApply := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update/apply", bytes.NewReader([]byte(`{}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", csrfCookie.Value)
		req.AddCookie(sessionCookie)
		req.AddCookie(csrfCookie)
		rec := httptest.NewRecorder()
		fx.router.ServeHTTP(rec, req)
		return rec
	}

	first := sendApply()
	if first.Code != http.StatusAccepted {
		t.Fatalf("expected first apply 202, got %d body=%s", first.Code, first.Body.String())
	}
	second := sendApply()
	if second.Code != http.StatusConflict {
		t.Fatalf("expected second apply 409, got %d body=%s", second.Code, second.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(second.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode second error body: %v body=%s", err, second.Body.String())
	}
	if apiErr.Code != "update_in_progress" {
		t.Fatalf("expected update_in_progress, got %q", apiErr.Code)
	}
}
