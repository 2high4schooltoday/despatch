package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/update"
	"despatch/internal/util"
)

func writeUpdaterUnitFilesForAPITest(t *testing.T, unitDir string) {
	t.Helper()
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	workerPath := filepath.Join(unitDir, "fake-update-worker")
	if err := os.WriteFile(workerPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake updater worker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "despatch-updater.path"), []byte("[Path]\nUnit=despatch-updater.service\n"), 0o644); err != nil {
		t.Fatalf("write updater path unit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "despatch-updater.service"), []byte("[Service]\nExecStart="+workerPath+"\n"), 0o644); err != nil {
		t.Fatalf("write updater service unit: %v", err)
	}
}

func installFakeSystemctlForAPITest(t *testing.T, pathLoad, pathActive, serviceLoad, serviceActive string) {
	installFakeSystemctlForAPITestWithDetails(t, pathLoad, pathActive, "", "", serviceLoad, serviceActive, "", "")
}

func installFakeSystemctlForAPITestWithDetails(t *testing.T, pathLoad, pathActive, pathSubState, pathResult, serviceLoad, serviceActive, serviceSubState, serviceResult string) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "systemctl")
	body := fmt.Sprintf(`#!/bin/sh
prop=""
unit=""
for arg in "$@"; do
  case "$arg" in
    --property=*) prop="${arg#--property=}" ;;
    show|--value) ;;
    *) unit="$arg" ;;
  esac
done
case "${unit}:${prop}" in
  despatch-updater.path:LoadState) printf '%%s\n' %q ;;
  despatch-updater.path:ActiveState) printf '%%s\n' %q ;;
  despatch-updater.path:SubState) printf '%%s\n' %q ;;
  despatch-updater.path:Result) printf '%%s\n' %q ;;
  despatch-updater.service:LoadState) printf '%%s\n' %q ;;
  despatch-updater.service:ActiveState) printf '%%s\n' %q ;;
  despatch-updater.service:SubState) printf '%%s\n' %q ;;
  despatch-updater.service:Result) printf '%%s\n' %q ;;
  *) printf '%%s\n' "" ;;
esac
`, pathLoad, pathActive, pathSubState, pathResult, serviceLoad, serviceActive, serviceSubState, serviceResult)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake systemctl: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

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
		writeUpdaterUnitFilesForAPITest(t, unitDir)
		installFakeSystemctlForAPITest(t, "loaded", "active", "loaded", "inactive")
	}

	cfg := config.Config{
		ListenAddr:             ":8080",
		BaseDomain:             "example.com",
		SessionCookieName:      "despatch_session",
		CSRFCookieName:         "despatch_csrf",
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
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          updateBase,
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	svc := service.New(cfg, st, nil, nil, nil)
	return updateTestFixture{
		router:     NewRouter(cfg, svc),
		store:      st,
		requestDir: filepath.Join(updateBase, "request"),
	}
}

func newUpdateFixtureWithRuntimeState(t *testing.T, enabled bool, pathLoad, pathActive, serviceLoad, serviceActive string) updateTestFixture {
	t.Helper()
	fx := newUpdateFixture(t, enabled, true)
	installFakeSystemctlForAPITest(t, pathLoad, pathActive, serviceLoad, serviceActive)
	return fx
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

func TestAdminUpdateStatusIncludesConfigDiagnosticWhenNotConfigured(t *testing.T) {
	fx := newUpdateFixture(t, true, false)
	sessionCookie, _ := loginCookies(t, fx.router, "admin@example.com", "SecretPass123!")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/update/status", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload update.StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Configured {
		t.Fatalf("expected configured=false when updater unit is missing")
	}
	if payload.ConfigDiagnostic == nil {
		t.Fatalf("expected config diagnostic for unconfigured updater")
	}
	if payload.ConfigDiagnostic.Reason != "updater_unit_missing" {
		t.Fatalf("expected updater_unit_missing reason, got %q", payload.ConfigDiagnostic.Reason)
	}
}

func TestAdminUpdateStatusIncludesAutomaticUpdatesByDefault(t *testing.T) {
	fx := newUpdateFixture(t, true, true)
	sessionCookie, _ := loginCookies(t, fx.router, "admin@example.com", "SecretPass123!")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/update/status", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	auto, _ := payload["auto_update"].(map[string]any)
	if enabled, _ := auto["enabled"].(bool); !enabled {
		t.Fatalf("expected auto_update.enabled default true, got %#v", auto["enabled"])
	}
}

func TestAdminUpdateAutomaticTogglePersists(t *testing.T) {
	fx := newUpdateFixture(t, true, true)
	sessionCookie, csrfCookie := loginCookies(t, fx.router, "admin@example.com", "SecretPass123!")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update/automatic", bytes.NewReader([]byte(`{"enabled":false}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/update/status", nil)
	statusReq.AddCookie(sessionCookie)
	statusRec := httptest.NewRecorder()
	fx.router.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", statusRec.Code, statusRec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(statusRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	auto, _ := payload["auto_update"].(map[string]any)
	if enabled, _ := auto["enabled"].(bool); enabled {
		t.Fatalf("expected auto_update.enabled false after toggle, got %#v", auto["enabled"])
	}
}

func TestAdminUpdateCancelScheduled(t *testing.T) {
	fx := newUpdateFixture(t, true, true)
	sessionCookie, csrfCookie := loginCookies(t, fx.router, "admin@example.com", "SecretPass123!")
	cfg := config.Config{
		UpdateEnabled:        true,
		UpdateBaseDir:        filepath.Dir(fx.requestDir),
		UpdateSystemdUnitDir: filepath.Join(filepath.Dir(filepath.Dir(fx.requestDir)), "units"),
	}
	if err := os.MkdirAll(filepath.Join(cfg.UpdateBaseDir, "status"), 0o770); err != nil {
		t.Fatalf("mkdir status dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.UpdateBaseDir, "status", "update-auto-status.json"), []byte("{\n  \"state\": \"scheduled\",\n  \"target_version\": \"v9.9.9\",\n  \"scheduled_for\": \"2030-01-01T02:00:00Z\"\n}\n"), 0o640); err != nil {
		t.Fatalf("write auto status: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/system/update/cancel-scheduled", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUpdateStatusOmitsConfigDiagnosticWhenConfigured(t *testing.T) {
	fx := newUpdateFixture(t, true, true)
	sessionCookie, _ := loginCookies(t, fx.router, "admin@example.com", "SecretPass123!")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/update/status", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload update.StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status payload: %v body=%s", err, rec.Body.String())
	}
	if !payload.Configured {
		t.Fatalf("expected configured=true when updater marker exists")
	}
	if payload.ConfigDiagnostic != nil {
		t.Fatalf("expected config diagnostic to be omitted when configured, got %#v", payload.ConfigDiagnostic)
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

func TestAdminUpdateStatusReportsInactiveUpdaterPathDiagnostic(t *testing.T) {
	fx := newUpdateFixtureWithRuntimeState(t, true, "loaded", "inactive", "loaded", "inactive")
	sessionCookie, _ := loginCookies(t, fx.router, "admin@example.com", "SecretPass123!")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/update/status", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload update.StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Configured {
		t.Fatalf("expected configured=false when updater path unit is inactive")
	}
	if payload.ConfigDiagnostic == nil || payload.ConfigDiagnostic.Reason != "updater_path_inactive" {
		t.Fatalf("expected updater_path_inactive diagnostic, got %#v", payload.ConfigDiagnostic)
	}
}

func TestAdminUpdateStatusReportsTriggerLimitedUpdaterPathDiagnostic(t *testing.T) {
	fx := newUpdateFixture(t, true, true)
	installFakeSystemctlForAPITestWithDetails(t, "loaded", "failed", "failed", "trigger-limit-hit", "loaded", "failed", "failed", "start-limit-hit")
	sessionCookie, _ := loginCookies(t, fx.router, "admin@example.com", "SecretPass123!")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/update/status", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	fx.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload update.StatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode status payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Configured {
		t.Fatalf("expected configured=false when updater path unit hit trigger limit")
	}
	if payload.ConfigDiagnostic == nil || payload.ConfigDiagnostic.Reason != "updater_path_trigger_limited" {
		t.Fatalf("expected updater_path_trigger_limited diagnostic, got %#v", payload.ConfigDiagnostic)
	}
}

func TestAdminUpdateApplyReturns503WhenUpdaterPathInactive(t *testing.T) {
	fx := newUpdateFixtureWithRuntimeState(t, true, "loaded", "inactive", "loaded", "inactive")
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
		t.Fatalf("decode error payload: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "updater_not_configured" {
		t.Fatalf("expected updater_not_configured, got %#v", apiErr)
	}
}
