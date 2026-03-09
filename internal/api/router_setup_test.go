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

	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/service"
	"despatch/internal/store"
)

func newSetupRouterWithConfigAndStore(t *testing.T, mutate func(*config.Config)) (http.Handler, *store.Store) {
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

	st := store.New(sqdb)
	cfg := config.Config{
		ListenAddr:                 ":8080",
		BaseDomain:                 "example.com",
		SessionCookieName:          "despatch_session",
		CSRFCookieName:             "despatch_csrf",
		SessionIdleMinutes:         30,
		SessionAbsoluteHour:        24,
		SessionEncryptKey:          "this_is_a_valid_long_session_encrypt_key_123456",
		CookieSecureMode:           "never",
		PasswordMinLength:          12,
		PasswordMaxLength:          128,
		DovecotAuthMode:            "sql",
		PasskeyPasswordlessEnabled: true,
		MailSecEnabled:             true,
		WebAuthnRPID:               "localhost",
	}
	if mutate != nil {
		mutate(&cfg)
	}
	svc := service.New(cfg, st, mail.NoopClient{}, mail.NoopProvisioner{}, nil)
	return NewRouter(cfg, svc), st
}

func TestSetupStatusIncludesPasskeyPrimarySignInEnabledAndAutomaticUpdates(t *testing.T) {
	router, _ := newSetupRouterWithConfigAndStore(t, nil)

	req := httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/setup/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if enabled, _ := payload["passkey_primary_sign_in_enabled"].(bool); !enabled {
		t.Fatalf("expected passkey_primary_sign_in_enabled=true, payload=%v", payload)
	}
	if enabled, ok := payload["automatic_updates_enabled"].(bool); !ok || !enabled {
		t.Fatalf("expected automatic_updates_enabled=true, payload=%v", payload)
	}
}

func TestSetupCompletePersistsPasskeyPrimarySignInChoice(t *testing.T) {
	router, st := newSetupRouterWithConfigAndStore(t, nil)

	body := []byte(`{
		"base_domain":"example.com",
		"admin_email":"webmaster@example.com",
		"admin_recovery_email":"recovery@example.net",
		"admin_password":"SecretPass123!",
		"region":"us-east",
		"passkey_primary_sign_in_enabled":false,
		"automatic_updates_enabled":false
	}`)
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/setup/complete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected setup complete 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	raw, ok, err := st.GetSetting(context.Background(), "feature_flag.passkey_sign_in")
	if err != nil {
		t.Fatalf("load passkey flag setting: %v", err)
	}
	if !ok || raw != "0" {
		t.Fatalf("expected feature_flag.passkey_sign_in=0, got ok=%v raw=%q", ok, raw)
	}
	autoRaw, autoOK, err := st.GetSetting(context.Background(), "update_auto_enabled")
	if err != nil {
		t.Fatalf("load auto update setting: %v", err)
	}
	if !autoOK || autoRaw != "0" {
		t.Fatalf("expected update_auto_enabled=0, got ok=%v raw=%q", autoOK, autoRaw)
	}
	user, err := st.GetUserByEmail(context.Background(), "webmaster@example.com")
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	if user.RecoveryEmail == nil || *user.RecoveryEmail != "recovery@example.net" {
		t.Fatalf("expected recovery email to persist, got %#v", user.RecoveryEmail)
	}

	capsReq := httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/public/auth/capabilities", nil)
	capsRec := httptest.NewRecorder()
	router.ServeHTTP(capsRec, capsReq)
	if capsRec.Code != http.StatusOK {
		t.Fatalf("expected caps 200, got %d body=%s", capsRec.Code, capsRec.Body.String())
	}
	var caps map[string]any
	if err := json.Unmarshal(capsRec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decode caps: %v body=%s", err, capsRec.Body.String())
	}
	if available, _ := caps["passkey_passwordless_available"].(bool); available {
		t.Fatalf("expected passkey_passwordless_available=false after setup override, payload=%v", caps)
	}
}

func TestSetupCompleteWithoutPasskeyChoicePreservesDefault(t *testing.T) {
	router, st := newSetupRouterWithConfigAndStore(t, nil)

	body := []byte(`{
		"base_domain":"example.com",
		"admin_email":"webmaster@example.com",
		"admin_recovery_email":"recovery@example.net",
		"admin_password":"SecretPass123!",
		"region":"us-east"
	}`)
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/setup/complete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected setup complete 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	raw, ok, err := st.GetSetting(context.Background(), "feature_flag.passkey_sign_in")
	if err != nil {
		t.Fatalf("load passkey flag setting: %v", err)
	}
	if !ok || raw != "1" {
		t.Fatalf("expected feature_flag.passkey_sign_in=1 when field omitted, got ok=%v raw=%q", ok, raw)
	}
	autoRaw, autoOK, err := st.GetSetting(context.Background(), "update_auto_enabled")
	if err != nil {
		t.Fatalf("load auto update setting: %v", err)
	}
	if !autoOK || autoRaw != "1" {
		t.Fatalf("expected update_auto_enabled=1 when field omitted, got ok=%v raw=%q", autoOK, autoRaw)
	}

	capsReq := httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/public/auth/capabilities", nil)
	capsRec := httptest.NewRecorder()
	router.ServeHTTP(capsRec, capsReq)
	if capsRec.Code != http.StatusOK {
		t.Fatalf("expected caps 200, got %d body=%s", capsRec.Code, capsRec.Body.String())
	}
	var caps map[string]any
	if err := json.Unmarshal(capsRec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decode caps: %v body=%s", err, capsRec.Body.String())
	}
	if available, _ := caps["passkey_passwordless_available"].(bool); !available {
		t.Fatalf("expected passkey_passwordless_available=true when setup field is omitted, payload=%v", caps)
	}
}

func TestSetupCompleteRejectsRecoveryEmailMatchingLogin(t *testing.T) {
	router, _ := newSetupRouterWithConfigAndStore(t, nil)

	body := []byte(`{
		"base_domain":"example.com",
		"admin_email":"webmaster@example.com",
		"admin_recovery_email":"webmaster@example.com",
		"admin_password":"SecretPass123!",
		"region":"us-east"
	}`)
	req := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/setup/complete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if code, _ := payload["code"].(string); code != "recovery_email_matches_login" {
		t.Fatalf("expected recovery_email_matches_login, got %v", payload)
	}
}
