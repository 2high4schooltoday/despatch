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

func newAdminRegistrationRouter(t *testing.T) (http.Handler, *store.Store) {
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
		DovecotAuthMode:     "sql",
	}

	svc := service.New(cfg, st, &sendTestMailClient{}, mail.NoopProvisioner{}, nil)
	return NewRouter(cfg, svc), st
}

func addPendingRegistration(t *testing.T, st *store.Store, email string) string {
	t.Helper()
	pwHash, err := auth.HashPassword("PendingPass123!")
	if err != nil {
		t.Fatalf("hash pending password: %v", err)
	}
	if _, err := st.CreateUser(context.Background(), email, pwHash, "user", models.UserPending); err != nil {
		t.Fatalf("create pending user: %v", err)
	}
	reg, err := st.CreateRegistration(context.Background(), email, "127.0.0.1", "ua-hash", true)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	return reg.ID
}

func doAdminRequest(t *testing.T, router http.Handler, method, path string, body []byte, sess, csrf *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(sess)
	req.AddCookie(csrf)
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		req.Header.Set("X-CSRF-Token", csrf.Value)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestAdminRegistrationsListUsesSnakeCaseFields(t *testing.T) {
	router, st := newAdminRegistrationRouter(t)
	regID := addPendingRegistration(t, st, "pending@example.com")
	sess, csrf := loginForSend(t, router)

	rec := doAdminRequest(t, router, http.MethodGet, "/api/v1/admin/registrations?status=pending&page=1&page_size=50", nil, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []struct {
			ID        string    `json:"id"`
			Email     string    `json:"email"`
			Status    string    `json:"status"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(payload.Items))
	}
	if payload.Items[0].ID != regID {
		t.Fatalf("expected id %q, got %q", regID, payload.Items[0].ID)
	}
	if payload.Items[0].Email != "pending@example.com" {
		t.Fatalf("expected email pending@example.com, got %q", payload.Items[0].Email)
	}
	if payload.Items[0].Status != "pending" {
		t.Fatalf("expected pending status, got %q", payload.Items[0].Status)
	}
}

func TestAdminApproveRegistrationFlow(t *testing.T) {
	router, st := newAdminRegistrationRouter(t)
	regID := addPendingRegistration(t, st, "approve@example.com")
	sess, csrf := loginForSend(t, router)

	rec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/registrations/"+regID+"/approve", []byte(`{}`), sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	reg, err := st.GetRegistrationByID(context.Background(), regID)
	if err != nil {
		t.Fatalf("load registration: %v", err)
	}
	if reg.Status != "approved" {
		t.Fatalf("expected registration status approved, got %q", reg.Status)
	}
	u, err := st.GetUserByEmail(context.Background(), "approve@example.com")
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if u.Status != models.UserActive {
		t.Fatalf("expected user status active, got %q", u.Status)
	}
}

func TestAdminRejectRegistrationFlow(t *testing.T) {
	router, st := newAdminRegistrationRouter(t)
	regID := addPendingRegistration(t, st, "reject@example.com")
	sess, csrf := loginForSend(t, router)

	rec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/registrations/"+regID+"/reject", []byte(`{"reason":"Nope"}`), sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	reg, err := st.GetRegistrationByID(context.Background(), regID)
	if err != nil {
		t.Fatalf("load registration: %v", err)
	}
	if reg.Status != "rejected" {
		t.Fatalf("expected registration status rejected, got %q", reg.Status)
	}
	_, err = st.GetUserByEmail(context.Background(), "reject@example.com")
	if err != store.ErrNotFound {
		t.Fatalf("expected rejected user to be deleted, got err=%v", err)
	}

	usersRec := doAdminRequest(t, router, http.MethodGet, "/api/v1/admin/users?page=1&page_size=100", nil, sess, csrf)
	if usersRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from users list, got %d body=%s", usersRec.Code, usersRec.Body.String())
	}
	var usersPayload struct {
		Items []struct {
			Email string `json:"email"`
		} `json:"items"`
	}
	if err := json.Unmarshal(usersRec.Body.Bytes(), &usersPayload); err != nil {
		t.Fatalf("decode users payload: %v body=%s", err, usersRec.Body.String())
	}
	for _, it := range usersPayload.Items {
		if it.Email == "reject@example.com" {
			t.Fatalf("rejected user must not appear in users list")
		}
	}
}

func TestAdminRegistrationsListSupportsSearchAndTotal(t *testing.T) {
	router, st := newAdminRegistrationRouter(t)
	_ = addPendingRegistration(t, st, "alpha@example.com")
	_ = addPendingRegistration(t, st, "beta@example.com")
	sess, csrf := loginForSend(t, router)

	rec := doAdminRequest(t, router, http.MethodGet, "/api/v1/admin/registrations?status=all&q=alpha&page=1&page_size=50&sort=email&order=asc", nil, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []struct {
			Email string `json:"email"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Total < 1 {
		t.Fatalf("expected total >= 1, got %d", payload.Total)
	}
	if len(payload.Items) != 1 || payload.Items[0].Email != "alpha@example.com" {
		t.Fatalf("unexpected filtered result: %+v", payload.Items)
	}
}

func TestAdminBulkRegistrationDecisionApprove(t *testing.T) {
	router, st := newAdminRegistrationRouter(t)
	regA := addPendingRegistration(t, st, "bulk-a@example.com")
	regB := addPendingRegistration(t, st, "bulk-b@example.com")
	sess, csrf := loginForSend(t, router)

	rec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/registrations/bulk/decision", []byte(`{"ids":["`+regA+`","`+regB+`"],"decision":"approve"}`), sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	reg1, err := st.GetRegistrationByID(context.Background(), regA)
	if err != nil {
		t.Fatalf("load reg A: %v", err)
	}
	reg2, err := st.GetRegistrationByID(context.Background(), regB)
	if err != nil {
		t.Fatalf("load reg B: %v", err)
	}
	if reg1.Status != "approved" || reg2.Status != "approved" {
		t.Fatalf("expected both approved, got %q and %q", reg1.Status, reg2.Status)
	}
}

func TestAdminAuditIncludesStableSummaryFields(t *testing.T) {
	router, st := newAdminRegistrationRouter(t)
	regID := addPendingRegistration(t, st, "audit@example.com")
	sess, csrf := loginForSend(t, router)

	approve := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/registrations/"+regID+"/approve", []byte(`{}`), sess, csrf)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve failed: %d body=%s", approve.Code, approve.Body.String())
	}

	rec := doAdminRequest(t, router, http.MethodGet, "/api/v1/admin/audit-log?page=1&page_size=20", nil, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []struct {
			Action         string `json:"action"`
			SummaryCode    string `json:"summary_code"`
			SummaryText    string `json:"summary_text"`
			SummaryVersion int    `json:"summary_version"`
			Severity       string `json:"severity"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Total == 0 || len(payload.Items) == 0 {
		t.Fatalf("expected audit entries")
	}
	first := payload.Items[0]
	if first.Action == "" || first.SummaryCode == "" || first.SummaryText == "" || first.SummaryVersion != 1 || first.Severity == "" {
		t.Fatalf("audit summary fields are incomplete: %+v", first)
	}
}

func TestAdminBulkUserActionSuspendUnsuspend(t *testing.T) {
	router, st := newAdminRegistrationRouter(t)
	pwHash, err := auth.HashPassword("UserPass123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u, err := st.CreateUser(context.Background(), "bulk-user@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sess, csrf := loginForSend(t, router)

	suspendBody := []byte(`{"ids":["` + u.ID + `"],"action":"suspend"}`)
	suspendRec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/users/bulk/action", suspendBody, sess, csrf)
	if suspendRec.Code != http.StatusOK {
		t.Fatalf("suspend failed: %d body=%s", suspendRec.Code, suspendRec.Body.String())
	}

	afterSuspend, err := st.GetUserByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("load suspended user: %v", err)
	}
	if afterSuspend.Status != models.UserSuspended {
		t.Fatalf("expected suspended status, got %q", afterSuspend.Status)
	}

	unsuspendBody := []byte(`{"ids":["` + u.ID + `"],"action":"unsuspend"}`)
	unsuspendRec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/users/bulk/action", unsuspendBody, sess, csrf)
	if unsuspendRec.Code != http.StatusOK {
		t.Fatalf("unsuspend failed: %d body=%s", unsuspendRec.Code, unsuspendRec.Body.String())
	}

	afterUnsuspend, err := st.GetUserByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("load unsuspended user: %v", err)
	}
	if afterUnsuspend.Status != models.UserActive {
		t.Fatalf("expected active status, got %q", afterUnsuspend.Status)
	}
}

func TestAdminRejectRegistrationWorksWhenPendingUserMissing(t *testing.T) {
	router, st := newAdminRegistrationRouter(t)
	reg, err := st.CreateRegistration(context.Background(), "missing-user@example.com", "127.0.0.1", "ua", true)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	sess, csrf := loginForSend(t, router)

	rec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/registrations/"+reg.ID+"/reject", []byte(`{"reason":"No account row"}`), sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	updated, err := st.GetRegistrationByID(context.Background(), reg.ID)
	if err != nil {
		t.Fatalf("load registration: %v", err)
	}
	if updated.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", updated.Status)
	}
}

func TestAdminRejectRegistrationReturnsUserStateConflict(t *testing.T) {
	router, st := newAdminRegistrationRouter(t)
	pwHash, err := auth.HashPassword("UserPass123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if _, err := st.CreateUser(context.Background(), "conflict@example.com", pwHash, "user", models.UserActive); err != nil {
		t.Fatalf("create active user: %v", err)
	}
	reg, err := st.CreateRegistration(context.Background(), "conflict@example.com", "127.0.0.1", "ua", true)
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	sess, csrf := loginForSend(t, router)

	rec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/registrations/"+reg.ID+"/reject", []byte(`{"reason":"Nope"}`), sess, csrf)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["code"] != "registration_user_state_conflict" {
		t.Fatalf("expected registration_user_state_conflict, got %v", payload["code"])
	}
}
