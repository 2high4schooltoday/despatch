package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/models"
	"despatch/internal/notify"
	"despatch/internal/pamreset"
	"despatch/internal/service"
	"despatch/internal/store"
)

func newResetRouter(t *testing.T, cfg config.Config) (http.Handler, *store.Store) {
	return newResetRouterWithSender(t, cfg, nil)
}

func newResetRouterWithSender(t *testing.T, cfg config.Config, sender notify.Sender) (http.Handler, *store.Store) {
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
		filepath.Join("..", "..", "migrations", "021_password_reset_token_reservations.sql"),
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
	svc := service.New(cfg, st, &sendTestDespatch{}, mail.NoopProvisioner{}, sender)
	return NewRouter(cfg, svc), st
}

type failingResetSender struct {
	err error
}

func (s failingResetSender) SendPasswordReset(ctx context.Context, toEmail, token string) error {
	_ = ctx
	_ = toEmail
	_ = token
	return s.err
}

type captureResetSender struct {
	calls int
	to    string
	token string
}

func (s *captureResetSender) SendPasswordReset(ctx context.Context, toEmail, token string) error {
	_ = ctx
	s.calls++
	s.to = toEmail
	s.token = token
	return nil
}

func startFakeResetHelper(t *testing.T, ok bool, code string) string {
	t.Helper()
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("despatch-pam-reset-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen fake helper: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	})
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				var header [4]byte
				if _, err := io.ReadFull(c, header[:]); err != nil {
					return
				}
				size := binary.BigEndian.Uint32(header[:])
				payload := make([]byte, size)
				if _, err := io.ReadFull(c, payload); err != nil {
					return
				}
				var req pamreset.Request
				if err := json.Unmarshal(payload, &req); err != nil {
					return
				}
				respBody, err := json.Marshal(pamreset.Response{
					RequestID: req.RequestID,
					OK:        ok,
					Code:      code,
				})
				if err != nil {
					return
				}
				var respHeader [4]byte
				binary.BigEndian.PutUint32(respHeader[:], uint32(len(respBody)))
				if _, err := c.Write(respHeader[:]); err != nil {
					return
				}
				_, _ = c.Write(respBody)
			}(conn)
		}
	}()
	return socketPath
}

func defaultResetTestConfig() config.Config {
	return config.Config{
		ListenAddr:                      ":8080",
		BaseDomain:                      "example.com",
		SessionCookieName:               "despatch_session",
		CSRFCookieName:                  "despatch_csrf",
		SessionIdleMinutes:              30,
		SessionAbsoluteHour:             24,
		SessionEncryptKey:               "this_is_a_valid_long_session_encrypt_key_123456",
		CookieSecureMode:                "never",
		TrustProxy:                      false,
		PasswordMinLength:               12,
		PasswordMaxLength:               128,
		DovecotAuthMode:                 "pam",
		PasswordResetSender:             "smtp",
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
		SenderAddress       string `json:"sender_address"`
		SenderStatus        string `json:"sender_status"`
		SenderReason        string `json:"sender_reason"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if payload.AuthMode != "pam" || payload.SelfServiceEnabled || !payload.AdminResetEnabled || payload.Delivery != "smtp" || payload.TokenTTLMinutes != 30 || payload.RequiresMappedLogin {
		t.Fatalf("unexpected capabilities payload: %+v", payload)
	}
	if payload.SenderStatus != "external" || payload.SenderAddress == "" || payload.SenderReason != "external_sender_unconfirmed" {
		t.Fatalf("unexpected sender diagnostics payload: %+v", payload)
	}
}

func TestPasswordResetRequestReturnsUnavailableWhenExternalSenderIsUnconfirmed(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.PAMResetHelperEnabled = true
	router, _ := newResetRouter(t, cfg)

	body, _ := json.Marshal(map[string]string{"email": "unknown@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unconfirmed external sender, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPasswordResetRequestReturnsGenericAcceptedWhenEnabled(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.PAMResetHelperEnabled = true
	cfg.PasswordResetExternalSenderReady = true
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

func TestPasswordResetRequestReturnsUnavailableWhenLogSenderConfigured(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.PAMResetHelperEnabled = true
	cfg.PasswordResetSender = "log"
	router, _ := newResetRouter(t, cfg)

	body, _ := json.Marshal(map[string]string{"email": "unknown@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when log sender is configured, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPasswordResetRequestReturnsUnavailableWhenSenderIsDegraded(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.DovecotAuthMode = "sql"
	cfg.PasswordResetSender = "smtp"
	router, _ := newResetRouter(t, cfg)

	body, _ := json.Marshal(map[string]string{"email": "unknown@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when sender is degraded, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPasswordResetRequestReturnsAcceptedOnDeliverySoftFailure(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.PAMResetHelperEnabled = true
	cfg.PasswordResetExternalSenderReady = true
	cfg.PasswordResetSender = "smtp"
	router, st := newResetRouterWithSender(t, cfg, failingResetSender{err: errors.New("smtp down")})

	pwHash, err := auth.HashPassword("Recoverable123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(context.Background(), "recover-soft@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(context.Background(), user.ID, "recover-soft-mail@example.net"); err != nil {
		t.Fatalf("set recovery email: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"email": "recover-soft@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 on soft delivery failure, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPasswordResetRequestAcceptsRecoveryEmailIdentifier(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.PAMResetHelperEnabled = true
	cfg.PasswordResetExternalSenderReady = true
	cfg.PasswordResetSender = "smtp"
	sender := &captureResetSender{}
	router, st := newResetRouterWithSender(t, cfg, sender)

	pwHash, err := auth.HashPassword("RecoverByIdentifier123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(context.Background(), "recover-id@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(context.Background(), user.ID, "recover-id-mail@example.net"); err != nil {
		t.Fatalf("set recovery email: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"email": "recover-id-mail@example.net"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 on recovery-identifier reset request, got %d body=%s", rec.Code, rec.Body.String())
	}
	if sender.calls != 1 {
		t.Fatalf("expected one reset send, got %d", sender.calls)
	}
	if sender.to != "recover-id-mail@example.net" {
		t.Fatalf("expected reset send to recovery email, got %q", sender.to)
	}
}

func TestAdminResetPasswordFallsBackToEmailInPAMMode(t *testing.T) {
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
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 (helper unavailable), got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["code"] != "password_reset_helper_unavailable" {
		t.Fatalf("expected password_reset_helper_unavailable, got %v", payload["code"])
	}
}

func TestPasswordResetConfirmReturnsHelperUnavailable(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.PAMResetHelperEnabled = true
	cfg.PasswordResetExternalSenderReady = true
	sender := &captureResetSender{}
	router, st := newResetRouterWithSender(t, cfg, sender)

	pwHash, err := auth.HashPassword("ResetFlow123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(context.Background(), "reset-flow@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(context.Background(), user.ID, "reset-flow-recovery@example.net"); err != nil {
		t.Fatalf("set recovery email: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"email": "reset-flow@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 from request flow, got %d body=%s", rec.Code, rec.Body.String())
	}
	if sender.token == "" {
		t.Fatalf("expected captured reset token")
	}

	confirmBody, _ := json.Marshal(map[string]string{
		"token":        sender.token,
		"new_password": "ChangedPassword123!",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/confirm", bytes.NewReader(confirmBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 from helper unavailable confirm, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if payload["code"] != "password_reset_helper_unavailable" {
		t.Fatalf("expected password_reset_helper_unavailable, got %+v", payload)
	}
}

func TestPasswordResetConfirmReturnsHelperFailed(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.PAMResetHelperEnabled = true
	cfg.PasswordResetExternalSenderReady = true
	cfg.PAMResetHelperSocket = startFakeResetHelper(t, false, pamreset.ProtocolCodeError)
	sender := &captureResetSender{}
	router, st := newResetRouterWithSender(t, cfg, sender)

	pwHash, err := auth.HashPassword("ResetFlow124!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(context.Background(), "reset-fail@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(context.Background(), user.ID, "reset-fail-recovery@example.net"); err != nil {
		t.Fatalf("set recovery email: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"email": "reset-fail@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 from request flow, got %d body=%s", rec.Code, rec.Body.String())
	}
	if sender.token == "" {
		t.Fatalf("expected captured reset token")
	}

	confirmBody, _ := json.Marshal(map[string]string{
		"token":        sender.token,
		"new_password": "ChangedPassword124!",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/password/reset/confirm", bytes.NewReader(confirmBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 from helper failed confirm, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if payload["code"] != "password_reset_helper_failed" {
		t.Fatalf("expected password_reset_helper_failed, got %+v", payload)
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
		if c.Name == "despatch_session" {
			sessionCookie = c
		}
		if c.Name == "despatch_csrf" {
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

func TestMeReturnsNeedsRecoveryEmailWhenSameAsLogin(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.DovecotAuthMode = "sql"
	router, st := newResetRouter(t, cfg)

	pwHash, err := auth.HashPassword("SameRecovery123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	_, err = st.CreateUser(context.Background(), "same@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessionCookie, _ := loginForResetTest(t, router, "same@example.com", "SameRecovery123!")

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
		t.Fatalf("expected needs_recovery_email=true for same-as-login recovery, got %+v", payload)
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

func TestMeUpdateRecoveryEmailRejectsLoginAddress(t *testing.T) {
	cfg := defaultResetTestConfig()
	cfg.DovecotAuthMode = "sql"
	router, st := newResetRouter(t, cfg)

	pwHash, err := auth.HashPassword("RecoverMe123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	_, err = st.CreateUser(context.Background(), "recover3@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessionCookie, csrfCookie := loginForResetTest(t, router, "recover3@example.com", "RecoverMe123!")

	body, _ := json.Marshal(map[string]string{"recovery_email": "recover3@example.com"})
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
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["code"] != "recovery_email_matches_login" {
		t.Fatalf("expected recovery_email_matches_login, got %+v", payload)
	}
}
