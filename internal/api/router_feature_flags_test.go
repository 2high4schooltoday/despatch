package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/models"
	"despatch/internal/service"
)

func TestAdminFeatureFlagsList(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.PasskeyPasswordlessEnabled = true
		cfg.PasswordResetPublicEnabled = true
		cfg.CaptchaEnabled = true
		cfg.UpdateEnabled = true
		cfg.UpdateRequireSignature = true
		cfg.PAMResetHelperEnabled = true
		cfg.PasswordResetRequireMappedLogin = true
	})
	sess, csrf := loginForSend(t, router)

	rec := doAdminRequest(t, router, http.MethodGet, "/api/v1/admin/system/feature-flags", nil, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []service.FeatureFlagState `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Items) < 9 {
		t.Fatalf("expected at least 9 feature flags, got %d", len(payload.Items))
	}
	byID := map[string]service.FeatureFlagState{}
	for _, item := range payload.Items {
		byID[item.ID] = item
	}
	passkeySignIn, ok := byID[service.FeatureFlagPasskeySignIn]
	if !ok {
		t.Fatalf("expected %s in payload", service.FeatureFlagPasskeySignIn)
	}
	if !passkeySignIn.Editable || passkeySignIn.RequiresRestart {
		t.Fatalf("unexpected passkey_sign_in mutability: %+v", passkeySignIn)
	}
	if _, ok := byID["passkey_account_discovery"]; ok {
		t.Fatalf("did not expect passkey_account_discovery to remain in feature flag payload")
	}
	readOnlyCaptcha, ok := byID[service.FeatureFlagRegistrationCaptchaRequired]
	if !ok {
		t.Fatalf("expected %s in payload", service.FeatureFlagRegistrationCaptchaRequired)
	}
	if readOnlyCaptcha.Editable || !readOnlyCaptcha.RequiresRestart {
		t.Fatalf("unexpected captcha flag mutability: %+v", readOnlyCaptcha)
	}
}

func TestAdminFeatureFlagUpdateAndResetAffectsAuthCapabilities(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.PasskeyPasswordlessEnabled = true
		cfg.MailSecEnabled = true
		cfg.WebAuthnRPID = "localhost"
	})
	sess, csrf := loginForSend(t, router)

	updateBody := []byte(`{"enabled":false}`)
	updateRec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/system/feature-flags/passkey_sign_in", updateBody, sess, csrf)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
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
		t.Fatalf("expected passkey_passwordless_available=false after feature flag override, payload=%v", caps)
	}
	if reason, _ := caps["reason"].(string); reason != "passwordless_disabled" {
		t.Fatalf("expected reason=passwordless_disabled, got %q payload=%v", reason, caps)
	}

	resetRec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/system/feature-flags/passkey_sign_in/reset", []byte(`{}`), sess, csrf)
	if resetRec.Code != http.StatusOK {
		t.Fatalf("expected reset 200, got %d body=%s", resetRec.Code, resetRec.Body.String())
	}

	capsReq2 := httptest.NewRequest(http.MethodGet, "http://localhost/api/v1/public/auth/capabilities", nil)
	capsRec2 := httptest.NewRecorder()
	router.ServeHTTP(capsRec2, capsReq2)
	if capsRec2.Code != http.StatusOK {
		t.Fatalf("expected caps 200 after reset, got %d body=%s", capsRec2.Code, capsRec2.Body.String())
	}
	var caps2 map[string]any
	if err := json.Unmarshal(capsRec2.Body.Bytes(), &caps2); err != nil {
		t.Fatalf("decode caps2: %v body=%s", err, capsRec2.Body.String())
	}
	if available, _ := caps2["passkey_passwordless_available"].(bool); !available {
		t.Fatalf("expected passkey_passwordless_available=true after reset, payload=%v", caps2)
	}
}

func TestAuthCapabilitiesAlwaysReportUsernamelessForCompatibility(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.PasskeyPasswordlessEnabled = true
		cfg.PasskeyUsernamelessEnabled = false
		cfg.MailSecEnabled = true
		cfg.WebAuthnRPID = "localhost"
	})
	if err := st.UpsertSetting(context.Background(), "feature_flag.passkey_account_discovery", "0"); err != nil {
		t.Fatalf("seed legacy discovery flag: %v", err)
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
	if enabled, _ := caps["passkey_usernameless_enabled"].(bool); !enabled {
		t.Fatalf("expected passkey_usernameless_enabled=true for compatibility payload=%v", caps)
	}
}

func TestAuthCapabilitiesExposeOriginDiagnosticsOnMismatch(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.PasskeyPasswordlessEnabled = true
		cfg.MailSecEnabled = true
		cfg.WebAuthnRPID = "example.com"
		cfg.WebAuthnAllowedOrigins = []string{"https://mail.example.com"}
	})

	capsReq := httptest.NewRequest(http.MethodGet, "https://app.example.com/api/v1/public/auth/capabilities", nil)
	capsRec := httptest.NewRecorder()
	router.ServeHTTP(capsRec, capsReq)
	if capsRec.Code != http.StatusOK {
		t.Fatalf("expected caps 200, got %d body=%s", capsRec.Code, capsRec.Body.String())
	}
	var caps map[string]any
	if err := json.Unmarshal(capsRec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decode caps: %v body=%s", err, capsRec.Body.String())
	}
	if reason, _ := caps["reason"].(string); reason != "origin_mismatch" {
		t.Fatalf("expected reason=origin_mismatch, got %q payload=%v", reason, caps)
	}
	if requestOrigin, _ := caps["request_origin"].(string); requestOrigin != "https://app.example.com" {
		t.Fatalf("expected request_origin=https://app.example.com, got %q payload=%v", requestOrigin, caps)
	}
	if rpID, _ := caps["rp_id"].(string); rpID != "example.com" {
		t.Fatalf("expected rp_id=example.com, got %q payload=%v", rpID, caps)
	}
	allowed, ok := caps["allowed_origins"].([]any)
	if !ok || len(allowed) != 1 || allowed[0] != "https://mail.example.com" {
		t.Fatalf("expected allowed_origins=[https://mail.example.com], payload=%v", caps)
	}
}

func TestAuthCapabilitiesHonorForwardedHostWhenTrustProxyEnabled(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.PasskeyPasswordlessEnabled = true
		cfg.MailSecEnabled = true
		cfg.TrustProxy = true
		cfg.WebAuthnRPID = "example.com"
		cfg.WebAuthnAllowedOrigins = []string{"https://mail.example.com"}
	})

	capsReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/v1/public/auth/capabilities", nil)
	capsReq.Header.Set("X-Forwarded-Proto", "https")
	capsReq.Header.Set("X-Forwarded-Host", "mail.example.com")
	capsRec := httptest.NewRecorder()
	router.ServeHTTP(capsRec, capsReq)
	if capsRec.Code != http.StatusOK {
		t.Fatalf("expected caps 200, got %d body=%s", capsRec.Code, capsRec.Body.String())
	}
	var caps map[string]any
	if err := json.Unmarshal(capsRec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decode caps: %v body=%s", err, capsRec.Body.String())
	}
	if available, _ := caps["passkey_mfa_available"].(bool); !available {
		t.Fatalf("expected passkey_mfa_available=true, payload=%v", caps)
	}
	if reason, _ := caps["reason"].(string); reason != "" {
		t.Fatalf("expected empty reason, got %q payload=%v", reason, caps)
	}
	if requestOrigin, _ := caps["request_origin"].(string); requestOrigin != "https://mail.example.com" {
		t.Fatalf("expected request_origin=https://mail.example.com, got %q payload=%v", requestOrigin, caps)
	}
}

func TestAdminFeatureFlagPasskeySignInAffectsLoginFlow(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.PasskeyPasswordlessEnabled = true
		cfg.MailSecEnabled = true
		cfg.WebAuthnRPID = "localhost"
		cfg.WebAuthnAllowedOrigins = []string{"http://localhost"}
	})
	sess, csrf := loginForSend(t, router)

	disableRec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/system/feature-flags/passkey_sign_in", []byte(`{"enabled":false}`), sess, csrf)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("expected disable 200, got %d body=%s", disableRec.Code, disableRec.Body.String())
	}

	beginReq := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/login/passkey/begin", bytes.NewReader([]byte(`{}`)))
	beginReq.Header.Set("Content-Type", "application/json")
	beginRec := httptest.NewRecorder()
	router.ServeHTTP(beginRec, beginReq)
	if beginRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for passkey begin when disabled, got %d body=%s", beginRec.Code, beginRec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(beginRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode passkey begin payload: %v body=%s", err, beginRec.Body.String())
	}
	if code, _ := payload["code"].(string); code != "passkey_unavailable" {
		t.Fatalf("expected passkey_unavailable code, got=%v payload=%v", code, payload)
	}
}

func TestAdminFeatureFlagReadOnlyRejected(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginForSend(t, router)

	body := []byte(`{"enabled":false}`)
	rec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/system/feature-flags/registration_captcha_required", body, sess, csrf)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if code, _ := payload["code"].(string); code != "feature_flag_read_only" {
		t.Fatalf("expected feature_flag_read_only, got %v payload=%v", code, payload)
	}
}

func TestAdminFeatureFlagEndpointRequiresAdmin(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	pwHash, err := auth.HashPassword("UserSecret123!")
	if err != nil {
		t.Fatalf("hash user password: %v", err)
	}
	if _, err := st.CreateUserWithMFA(context.Background(), "user@example.com", pwHash, "user", models.UserActive, "none"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	sess, csrf := loginForResetTest(t, router, "user@example.com", "UserSecret123!")

	rec := doAdminRequest(t, router, http.MethodGet, "/api/v1/admin/system/feature-flags", nil, sess, csrf)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPublicPasswordResetRuntimeFlag(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.PasswordResetPublicEnabled = true
		cfg.DovecotAuthMode = "sql"
		cfg.PasswordResetSender = "log"
	})
	sess, csrf := loginForSend(t, router)

	disableRec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/system/feature-flags/public_password_reset", []byte(`{"enabled":false}`), sess, csrf)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("expected 200 while disabling, got %d body=%s", disableRec.Code, disableRec.Body.String())
	}

	capsReq := httptest.NewRequest(http.MethodGet, "/api/v1/public/password-reset/capabilities", nil)
	capsRec := httptest.NewRecorder()
	router.ServeHTTP(capsRec, capsReq)
	if capsRec.Code != http.StatusOK {
		t.Fatalf("expected caps 200, got %d body=%s", capsRec.Code, capsRec.Body.String())
	}
	var caps map[string]any
	if err := json.Unmarshal(capsRec.Body.Bytes(), &caps); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if delivery, _ := caps["delivery"].(string); delivery != "disabled" {
		t.Fatalf("expected delivery=disabled, got %q payload=%v", delivery, caps)
	}
	if enabled, _ := caps["self_service_enabled"].(bool); enabled {
		t.Fatalf("expected self_service_enabled=false when flag disabled, payload=%v", caps)
	}

	resetReqBody, _ := json.Marshal(map[string]string{"email": "admin@example.com"})
	resetReq := httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/request", bytes.NewReader(resetReqBody))
	resetReq.Header.Set("Content-Type", "application/json")
	resetRec := httptest.NewRecorder()
	router.ServeHTTP(resetRec, resetReq)
	if resetRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected reset request 503 when feature disabled, got %d body=%s", resetRec.Code, resetRec.Body.String())
	}
}

func TestAdminFeatureFlagUpdateWritesAuditEntry(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginForSend(t, router)

	updateRec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/system/feature-flags/passkey_sign_in", []byte(`{"enabled":false}`), sess, csrf)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	auditRec := doAdminRequest(t, router, http.MethodGet, "/api/v1/admin/audit-log?action=feature_flag.update&page=1&page_size=20", nil, sess, csrf)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	var payload struct {
		Items []struct {
			Action string `json:"action"`
			Target string `json:"target"`
		} `json:"items"`
	}
	if err := json.Unmarshal(auditRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode audit payload: %v body=%s", err, auditRec.Body.String())
	}
	if len(payload.Items) == 0 {
		t.Fatalf("expected audit entries for feature_flag.update")
	}
	found := false
	for _, item := range payload.Items {
		if item.Action == "feature_flag.update" && item.Target == "passkey_sign_in" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected feature_flag.update entry for passkey_sign_in, got=%+v", payload.Items)
	}
}
