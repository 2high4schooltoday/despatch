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
	"mailclient/internal/util"
)

func newCaptchaRouter(t *testing.T, cfg config.Config) http.Handler {
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

	svc := service.New(cfg, st, nil, nil, nil)
	return NewRouter(cfg, svc)
}

func baseCaptchaConfig() config.Config {
	return config.Config{
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
}

func TestPublicCaptchaConfigCAP(t *testing.T) {
	cfg := baseCaptchaConfig()
	cfg.CaptchaEnabled = true
	cfg.CaptchaProvider = "cap"
	cfg.CaptchaSiteKey = "cap-site-key-123"
	cfg.CaptchaWidgetURL = "/cap/cap-site-key-123/"
	cfg.CaptchaVerifyURL = "http://127.0.0.1:8077/cap-site-key-123/siteverify"
	cfg.CaptchaSecret = "cap-secret"
	router := newCaptchaRouter(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/captcha/config", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Enabled      bool   `json:"enabled"`
		Provider     string `json:"provider"`
		SiteKey      string `json:"site_key"`
		WidgetAPIURL string `json:"widget_api_url"`
		Mode         string `json:"mode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
	}
	if !payload.Enabled {
		t.Fatalf("expected enabled=true")
	}
	if payload.Provider != "cap" {
		t.Fatalf("expected provider=cap, got %q", payload.Provider)
	}
	if payload.SiteKey != "cap-site-key-123" {
		t.Fatalf("unexpected site key: %q", payload.SiteKey)
	}
	if payload.WidgetAPIURL != "/cap/cap-site-key-123/" {
		t.Fatalf("unexpected widget_api_url: %q", payload.WidgetAPIURL)
	}
	if payload.Mode != "required" {
		t.Fatalf("expected mode=required, got %q", payload.Mode)
	}
}

func TestRegisterCaptchaMissingTokenReturnsCaptchaRequired(t *testing.T) {
	verifyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer verifyServer.Close()

	cfg := baseCaptchaConfig()
	cfg.CaptchaEnabled = true
	cfg.CaptchaProvider = "cap"
	cfg.CaptchaSiteKey = "cap-site-key-123"
	cfg.CaptchaWidgetURL = "/cap/cap-site-key-123/"
	cfg.CaptchaVerifyURL = verifyServer.URL
	cfg.CaptchaSecret = "cap-secret"
	router := newCaptchaRouter(t, cfg)

	body, _ := json.Marshal(map[string]string{
		"email":         "new-user@example.com",
		"password":      "SecretPass123!",
		"captcha_token": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode body: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "captcha_required" {
		t.Fatalf("expected code captcha_required, got %q body=%s", apiErr.Code, rec.Body.String())
	}
}

func TestRegisterCaptchaVerifierUnavailableReturns503(t *testing.T) {
	verifyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream unavailable"}`))
	}))
	defer verifyServer.Close()

	cfg := baseCaptchaConfig()
	cfg.CaptchaEnabled = true
	cfg.CaptchaProvider = "cap"
	cfg.CaptchaSiteKey = "cap-site-key-123"
	cfg.CaptchaWidgetURL = "/cap/cap-site-key-123/"
	cfg.CaptchaVerifyURL = verifyServer.URL
	cfg.CaptchaSecret = "cap-secret"
	router := newCaptchaRouter(t, cfg)

	body, _ := json.Marshal(map[string]string{
		"email":         "new-user2@example.com",
		"password":      "SecretPass123!",
		"captcha_token": "token-ok",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode body: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "captcha_unavailable" {
		t.Fatalf("expected code captcha_unavailable, got %q body=%s", apiErr.Code, rec.Body.String())
	}
}
