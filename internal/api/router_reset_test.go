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
	"mailclient/internal/mail"
	"mailclient/internal/models"
	"mailclient/internal/service"
	"mailclient/internal/store"
)

func newResetRouter(t *testing.T, cfg config.Config) (http.Handler, *store.Store) {
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
	svc := service.New(cfg, st, &sendTestMailClient{}, mail.NoopProvisioner{}, nil)
	return NewRouter(cfg, svc), st
}

func defaultResetTestConfig() config.Config {
	return config.Config{
		ListenAddr:                      ":8080",
		BaseDomain:                      "example.com",
		SessionCookieName:               "mailclient_session",
		CSRFCookieName:                  "mailclient_csrf",
		SessionIdleMinutes:              30,
		SessionAbsoluteHour:             24,
		SessionEncryptKey:               "this_is_a_valid_long_session_encrypt_key_123456",
		CookieSecureMode:                "never",
		TrustProxy:                      false,
		PasswordMinLength:               12,
		PasswordMaxLength:               128,
		DovecotAuthMode:                 "pam",
		PasswordResetSender:             "log",
		PasswordResetTokenTTLMinutes:    30,
		PasswordResetPublicEnabled:      true,
		PasswordResetRequireMappedLogin: true,
		PAMResetHelperEnabled:           false,
		PAMResetHelperTimeoutSec:        5,
		PAMResetAllowedUID:              -1,
		PAMResetAllowedGID:              -1,
		PAMResetHelperSocket:            "/tmp/nonexistent.sock",
	}
}

func TestPublicPasswordResetCapabilities(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.PAMResetHelperEnabled = true
	router, _ := newResetRouter(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public/password-reset/capabilities", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		AuthMode            string `json:"auth_mode"`
		SelfServiceEnabled  bool   `json:"self_service_enabled"`
		AdminResetEnabled   bool   `json:"admin_reset_enabled"`
		Delivery            string `json:"delivery"`
		TokenTTLMinutes     int    `json:"token_ttl_minutes"`
		RequiresMappedLogin bool   `json:"requires_mapped_login"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if payload.AuthMode != "pam" || !payload.SelfServiceEnabled || !payload.AdminResetEnabled || payload.Delivery != "log" || payload.TokenTTLMinutes != 30 || !payload.RequiresMappedLogin {
		t.Fatalf("unexpected capabilities payload: %+v", payload)
	}
}

func TestPasswordResetRequestReturnsGenericAcceptedWhenEnabled(t *testing.T) {
	cfg := defaultResetTestConfig()
	router, _ := newResetRouter(t, cfg)

	body, _ := json.Marshal(map[string]string{"email": "unknown@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPasswordResetRequestReturnsUnavailableWhenDisabled(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.PasswordResetPublicEnabled = false
	router, _ := newResetRouter(t, cfg)

	body, _ := json.Marshal(map[string]string{"email": "unknown@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminResetPasswordReturnsMappedLoginErrorInPAMMode(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.PAMResetHelperEnabled = true
	router, st := newResetRouter(t, cfg)

	pwHash, err := auth.HashPassword("UserPass123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(context.Background(), "nomap@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sess, csrf := loginForSend(t, router)
	body, _ := json.Marshal(map[string]string{"new_password": "NewPassword123!"})
	rec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/users/"+user.ID+"/reset-password", body, sess, csrf)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["code"] != "password_reset_login_unmapped" {
		t.Fatalf("expected password_reset_login_unmapped, got %v", payload["code"])
	}
}

func loginForResetTest(t *testing.T, router http.Handler, email, password string) (*http.Cookie, *http.Cookie) {
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

func TestMeReturnsNeedsRecoveryEmailWhenMissing(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.DovecotAuthMode = "sql"
	router, st := newResetRouter(t, cfg)

	pwHash, err := auth.HashPassword("NoRecovery123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(context.Background(), "legacy@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(context.Background(), user.ID, ""); err != nil {
		t.Fatalf("clear recovery email: %v", err)
	}
	sessionCookie, _ := loginForResetTest(t, router, "legacy@example.com", "NoRecovery123!")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected /me 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["needs_recovery_email"] != true {
		t.Fatalf("expected needs_recovery_email=true, got %+v", payload)
	}
}

func TestMeUpdateRecoveryEmail(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.DovecotAuthMode = "sql"
	router, st := newResetRouter(t, cfg)

	pwHash, err := auth.HashPassword("RecoverMe123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(context.Background(), "recover@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(context.Background(), user.ID, ""); err != nil {
		t.Fatalf("clear recovery email: %v", err)
	}
	sessionCookie, csrfCookie := loginForResetTest(t, router, "recover@example.com", "RecoverMe123!")

	body, _ := json.Marshal(map[string]string{"recovery_email": "new-recovery@example.net"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/recovery-email", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	updated, err := st.GetUserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("load updated user: %v", err)
	}
	if updated.RecoveryEmail == nil || *updated.RecoveryEmail != "new-recovery@example.net" {
		t.Fatalf("expected recovery email to be saved, got %+v", updated.RecoveryEmail)
	}
}

func TestMeUpdateRecoveryEmailRejectsInvalidAddress(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.DovecotAuthMode = "sql"
	router, st := newResetRouter(t, cfg)

	pwHash, err := auth.HashPassword("RecoverMe123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	_, err = st.CreateUser(context.Background(), "recover2@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessionCookie, csrfCookie := loginForResetTest(t, router, "recover2@example.com", "RecoverMe123!")

	body, _ := json.Marshal(map[string]string{"recovery_email": "not-an-email"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/me/recovery-email", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected update 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
