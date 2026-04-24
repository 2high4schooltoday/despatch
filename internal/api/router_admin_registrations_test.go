package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/util"
)

func newAdminRegistrationRouter(t *testing.T) (http.Handler, *store.Store) {
	router, st, _ := newAdminRegistrationRouterWithOptions(t, nil, &sendTestDespatch{}, mail.NoopProvisioner{})
	return router, st
}

func newAdminRegistrationRouterWithDB(t *testing.T) (http.Handler, *store.Store, *sql.DB) {
	return newAdminRegistrationRouterWithOptions(t, nil, &sendTestDespatch{}, mail.NoopProvisioner{})
}

func newAdminRegistrationRouterWithOptions(t *testing.T, configure func(*config.Config), client mail.Client, provisioner mail.AuthProvisioner) (http.Handler, *store.Store, *sql.DB) {
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
		SessionCookieName:   "despatch_session",
		CSRFCookieName:      "despatch_csrf",
		SessionIdleMinutes:  30,
		SessionAbsoluteHour: 24,
		SessionEncryptKey:   "this_is_a_valid_long_session_encrypt_key_123456",
		CookieSecureMode:    "never",
		TrustProxy:          false,
		PasswordMinLength:   12,
		PasswordMaxLength:   128,
		DovecotAuthMode:     "sql",
	}
	if configure != nil {
		configure(&cfg)
	}
	if client == nil {
		client = &sendTestDespatch{}
	}
	if provisioner == nil {
		provisioner = mail.NoopProvisioner{}
	}

	svc := service.New(cfg, st, client, provisioner, nil)
	return NewRouter(cfg, svc), st, sqdb
}

type captureAdminProvisioner struct {
	upserts  []string
	disabled []string
}

func (c *captureAdminProvisioner) UpsertActiveUser(ctx context.Context, email, passwordHash string) error {
	_ = ctx
	_ = passwordHash
	c.upserts = append(c.upserts, email)
	return nil
}

func (c *captureAdminProvisioner) DisableUser(ctx context.Context, email string) error {
	_ = ctx
	c.disabled = append(c.disabled, email)
	return nil
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

func TestAdminCreateUserProvisionsActiveAccount(t *testing.T) {
	provisioner := &captureAdminProvisioner{}
	router, st, _ := newAdminRegistrationRouterWithOptions(t, nil, &sendTestDespatch{}, provisioner)
	sess, csrf := loginForSend(t, router)

	rec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/users", []byte(`{
		"email":"new.user@example.com",
		"recovery_email":"new.user@personal.test",
		"password":"NewUserPass123!"
	}`), sess, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	user, err := st.GetUserByEmail(context.Background(), "new.user@example.com")
	if err != nil {
		t.Fatalf("load created user: %v", err)
	}
	if user.Status != models.UserActive {
		t.Fatalf("expected active status, got %q", user.Status)
	}
	if user.ProvisionState != "ok" {
		t.Fatalf("expected provision ok, got %q", user.ProvisionState)
	}
	if user.RecoveryEmail == nil || *user.RecoveryEmail != "new.user@personal.test" {
		t.Fatalf("expected recovery email to persist, got %+v", user.RecoveryEmail)
	}
	if len(provisioner.upserts) != 1 || provisioner.upserts[0] != "new.user@example.com" {
		t.Fatalf("expected provisioner upsert call, got %+v", provisioner.upserts)
	}
}

func TestAdminCreateUserInPAMModePersistsAcceptedMailboxLogin(t *testing.T) {
	router, st, _ := newAdminRegistrationRouterWithOptions(t, func(cfg *config.Config) {
		cfg.DovecotAuthMode = "pam"
		cfg.IMAPHost = "127.0.0.1"
		cfg.IMAPPort = 993
		cfg.IMAPTLS = true
		cfg.SMTPHost = "127.0.0.1"
		cfg.SMTPPort = 25
		cfg.SMTPTLS = false
		cfg.SMTPStartTLS = false
	}, pamLoginTestDespatch{
		acceptPassword: "SecretPass123!",
		acceptedUsers: map[string]bool{
			"admin":   true,
			"newuser": true,
		},
	}, mail.NoopProvisioner{})
	sess, csrf := loginForSend(t, router)

	rec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/users", []byte(`{
		"email":"newuser@example.com",
		"password":"SecretPass123!"
	}`), sess, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	user, err := st.GetUserByEmail(context.Background(), "newuser@example.com")
	if err != nil {
		t.Fatalf("load created pam user: %v", err)
	}
	if user.Status != models.UserActive {
		t.Fatalf("expected active status, got %q", user.Status)
	}
	if user.ProvisionState != "ok" {
		t.Fatalf("expected provision ok, got %q", user.ProvisionState)
	}
	if user.MailLogin == nil || *user.MailLogin != "newuser" {
		t.Fatalf("expected accepted PAM mailbox login, got %+v", user.MailLogin)
	}
	accounts, err := st.ListMailAccounts(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("list bootstrapped mail accounts: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Login != "newuser" {
		t.Fatalf("expected one bootstrapped account for newuser, got %+v", accounts)
	}
}

func TestAdminMailboxCRUDWithStoredUserCredentials(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Messages: 3, Unread: 1},
			{Name: "Trash", Role: "trash"},
		},
	}
	router, st, _ := newAdminRegistrationRouterWithOptions(t, nil, client, mail.NoopProvisioner{})
	sess, csrf := loginForSend(t, router)

	pwHash, err := auth.HashPassword("MailboxPass123!")
	if err != nil {
		t.Fatalf("hash managed user password: %v", err)
	}
	user, err := st.CreateUser(context.Background(), "managed@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create managed user: %v", err)
	}
	secret, err := util.EncryptString(util.Derive32ByteKey("this_is_a_valid_long_session_encrypt_key_123456"), "mailbox-secret")
	if err != nil {
		t.Fatalf("encrypt mailbox secret: %v", err)
	}
	if err := st.UpsertUserMailSecret(context.Background(), user.ID, secret); err != nil {
		t.Fatalf("store mailbox secret: %v", err)
	}

	listRec := doAdminRequest(t, router, http.MethodGet, "/api/v1/admin/users/"+user.ID+"/mailboxes", nil, sess, csrf)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listPayload struct {
		Items []mail.Mailbox `json:"items"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listPayload); err != nil {
		t.Fatalf("decode list payload: %v body=%s", err, listRec.Body.String())
	}
	if len(listPayload.Items) != 2 {
		t.Fatalf("expected 2 mailboxes, got %d", len(listPayload.Items))
	}

	createRec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/users/"+user.ID+"/mailboxes", []byte(`{"mailbox_name":"Projects"}`), sess, csrf)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if len(client.createdMailboxes) != 1 || client.createdMailboxes[0] != "Projects" {
		t.Fatalf("expected Projects mailbox creation, got %+v", client.createdMailboxes)
	}
	var createPayload struct {
		MailboxName string         `json:"mailbox_name"`
		Mailboxes   []mail.Mailbox `json:"mailboxes"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createPayload); err != nil {
		t.Fatalf("decode create payload: %v body=%s", err, createRec.Body.String())
	}
	if createPayload.MailboxName != "Projects" {
		t.Fatalf("expected created mailbox Projects, got %q", createPayload.MailboxName)
	}
	foundCreated := false
	for _, item := range createPayload.Mailboxes {
		if item.Name == "Projects" {
			foundCreated = true
			break
		}
	}
	if !foundCreated {
		t.Fatalf("expected Projects in response mailboxes, got %+v", createPayload.Mailboxes)
	}

	deleteRec := doAdminRequest(t, router, http.MethodDelete, "/api/v1/admin/users/"+user.ID+"/mailboxes", []byte(`{"mailbox_name":"Projects"}`), sess, csrf)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	if len(client.deletedMailboxes) != 1 || client.deletedMailboxes[0] != "Projects" {
		t.Fatalf("expected Projects mailbox deletion, got %+v", client.deletedMailboxes)
	}
	var deletePayload struct {
		MailboxName string         `json:"mailbox_name"`
		Mailboxes   []mail.Mailbox `json:"mailboxes"`
	}
	if err := json.Unmarshal(deleteRec.Body.Bytes(), &deletePayload); err != nil {
		t.Fatalf("decode delete payload: %v body=%s", err, deleteRec.Body.String())
	}
	if deletePayload.MailboxName != "Projects" {
		t.Fatalf("expected deleted mailbox Projects, got %q", deletePayload.MailboxName)
	}
	for _, item := range deletePayload.Mailboxes {
		if item.Name == "Projects" {
			t.Fatalf("expected Projects to be removed, got %+v", deletePayload.Mailboxes)
		}
	}
}

func TestAdminMailboxListRequiresStoredCredentials(t *testing.T) {
	router, st, _ := newAdminRegistrationRouterWithOptions(t, nil, &mailRouterTestClient{}, mail.NoopProvisioner{})
	sess, csrf := loginForSend(t, router)

	pwHash, err := auth.HashPassword("MailboxPass123!")
	if err != nil {
		t.Fatalf("hash managed user password: %v", err)
	}
	user, err := st.CreateUser(context.Background(), "nocreds@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create managed user without credentials: %v", err)
	}

	rec := doAdminRequest(t, router, http.MethodGet, "/api/v1/admin/users/"+user.ID+"/mailboxes", nil, sess, csrf)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode api error: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "mailbox_credentials_unavailable" {
		t.Fatalf("expected mailbox_credentials_unavailable, got %q body=%s", apiErr.Code, rec.Body.String())
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

func TestAdminRejectRegistrationSucceedsWhenPreviousRejectedExistsForSameEmail(t *testing.T) {
	router, st := newAdminRegistrationRouter(t)
	sess, csrf := loginForSend(t, router)
	email := "repeat-reject@example.com"

	firstID := addPendingRegistration(t, st, email)
	firstReject := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/registrations/"+firstID+"/reject", []byte(`{"reason":"first pass"}`), sess, csrf)
	if firstReject.Code != http.StatusOK {
		t.Fatalf("first reject failed: %d body=%s", firstReject.Code, firstReject.Body.String())
	}

	secondID := addPendingRegistration(t, st, email)
	secondReject := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/registrations/"+secondID+"/reject", []byte(`{"reason":"second pass"}`), sess, csrf)
	if secondReject.Code != http.StatusOK {
		t.Fatalf("second reject failed: %d body=%s", secondReject.Code, secondReject.Body.String())
	}

	updated, err := st.GetRegistrationByID(context.Background(), secondID)
	if err != nil {
		t.Fatalf("load second registration: %v", err)
	}
	if updated.Status != "rejected" {
		t.Fatalf("expected second registration rejected, got %q", updated.Status)
	}

	_, err = st.GetRegistrationByID(context.Background(), firstID)
	if err != store.ErrNotFound {
		t.Fatalf("expected first rejected row to be replaced, got err=%v", err)
	}
}

func TestAdminBulkRejectRegistrationSucceedsWhenAuditInsertFails(t *testing.T) {
	router, st, sqdb := newAdminRegistrationRouterWithDB(t)
	regID := addPendingRegistration(t, st, "audit-failure@example.com")
	if _, err := sqdb.Exec(`DROP TABLE admin_audit_log`); err != nil {
		t.Fatalf("drop audit table: %v", err)
	}
	sess, csrf := loginForSend(t, router)

	body := []byte(`{"ids":["` + regID + `"],"decision":"reject","reason":"policy"}`)
	rec := doAdminRequest(t, router, http.MethodPost, "/api/v1/admin/registrations/bulk/decision", body, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Applied []string         `json:"applied"`
		Failed  []map[string]any `json:"failed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Failed) != 0 {
		t.Fatalf("expected no failed items, got %+v", payload.Failed)
	}
	if len(payload.Applied) != 1 || payload.Applied[0] != regID {
		t.Fatalf("expected applied to include %s, got %+v", regID, payload.Applied)
	}

	updated, err := st.GetRegistrationByID(context.Background(), regID)
	if err != nil {
		t.Fatalf("load registration: %v", err)
	}
	if updated.Status != "rejected" {
		t.Fatalf("expected rejected status, got %q", updated.Status)
	}
}
