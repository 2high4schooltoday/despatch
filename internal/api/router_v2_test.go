package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/mailsec"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
)

func newV2Router(t *testing.T) http.Handler {
	return newV2RouterWithMailClient(t, mail.NoopClient{}, nil)
}

func newV2RouterWithConfig(t *testing.T, mutate func(*config.Config)) http.Handler {
	router, _ := newV2RouterWithMailClientAndStore(t, mail.NoopClient{}, mutate)
	return router
}

func newV2RouterWithConfigAndStore(t *testing.T, mutate func(*config.Config)) (http.Handler, *store.Store) {
	router, st, _ := newV2RouterWithMailClientAndStoreDB(t, mail.NoopClient{}, mutate)
	return router, st
}

func newV2RouterWithConfigAndStoreDB(t *testing.T, mutate func(*config.Config)) (http.Handler, *store.Store, *sql.DB) {
	return newV2RouterWithMailClientAndStoreDB(t, mail.NoopClient{}, mutate)
}

func newV2RouterWithMailClient(t *testing.T, despatch mail.Client, mutate func(*config.Config)) http.Handler {
	router, _ := newV2RouterWithMailClientAndStore(t, despatch, mutate)
	return router
}

func newV2RouterWithMailClientAndStore(t *testing.T, despatch mail.Client, mutate func(*config.Config)) (http.Handler, *store.Store) {
	router, st, _ := newV2RouterWithMailClientAndStoreDB(t, despatch, mutate)
	return router, st
}

func newV2RouterWithMailClientAndStoreDB(t *testing.T, despatch mail.Client, mutate func(*config.Config)) (http.Handler, *store.Store, *sql.DB) {
	return newV2RouterWithMailClientStoreAndHook(t, despatch, mutate, nil)
}

func newV2RouterWithMailClientStoreAndHook(t *testing.T, despatch mail.Client, mutate func(*config.Config), hook func(*service.Service, *store.Store, *sql.DB, *config.Config)) (http.Handler, *store.Store, *sql.DB) {
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
		filepath.Join("..", "..", "migrations", "025_session_mail_profiles.sql"),
		filepath.Join("..", "..", "migrations", "026_draft_context_account.sql"),
		filepath.Join("..", "..", "migrations", "027_sender_profiles.sql"),
		filepath.Join("..", "..", "migrations", "028_contacts.sql"),
		filepath.Join("..", "..", "migrations", "029_mail_rules.sql"),
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
		ListenAddr:                 ":8080",
		BaseDomain:                 "example.com",
		SessionCookieName:          "despatch_session",
		CSRFCookieName:             "despatch_csrf",
		SessionIdleMinutes:         30,
		SessionAbsoluteHour:        24,
		SessionEncryptKey:          "this_is_a_valid_long_session_encrypt_key_123456",
		CookieSecureMode:           "never",
		TrustProxy:                 false,
		PasswordMinLength:          12,
		PasswordMaxLength:          128,
		DovecotAuthMode:            "sql",
		PasskeyPasswordlessEnabled: true,
		PasskeyUsernamelessEnabled: true,
		IMAPHost:                   "127.0.0.1",
		IMAPPort:                   993,
		IMAPTLS:                    true,
		SMTPHost:                   "127.0.0.1",
		SMTPPort:                   587,
		SMTPStartTLS:               true,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	svc := service.New(cfg, st, despatch, mail.NoopProvisioner{}, nil)
	if hook != nil {
		hook(svc, st, sqdb, &cfg)
	}
	return NewRouter(cfg, svc), st, sqdb
}

type fakeMailSecRecorder struct {
	mu       sync.Mutex
	requests []mailsec.Request
}

func (r *fakeMailSecRecorder) add(req mailsec.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
}

func (r *fakeMailSecRecorder) byOp(op string) []mailsec.Request {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]mailsec.Request, 0, len(r.requests))
	for _, req := range r.requests {
		if req.Op == op {
			out = append(out, req)
		}
	}
	return out
}

func startFakeMailSecServer(t *testing.T) string {
	socketPath, _ := startFakeMailSecServerWithRecorder(t)
	return socketPath
}

func startFakeMailSecServerWithRecorder(t *testing.T) (string, *fakeMailSecRecorder) {
	t.Helper()
	socketPath := filepath.Join("/tmp", fmt.Sprintf("mailsec-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen fake mailsec: %v", err)
	}
	recorder := &fakeMailSecRecorder{}
	t.Cleanup(func() {
		_ = ln.Close()
		_ = os.Remove(socketPath)
	})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				lenBuf := make([]byte, 4)
				if _, err := io.ReadFull(c, lenBuf); err != nil {
					return
				}
				n := int(binary.BigEndian.Uint32(lenBuf))
				if n <= 0 || n > 1024*1024 {
					return
				}
				payload := make([]byte, n)
				if _, err := io.ReadFull(c, payload); err != nil {
					return
				}
				var req mailsec.Request
				if err := json.Unmarshal(payload, &req); err != nil {
					return
				}
				recorder.add(req)
				resp := mailsec.Response{
					RequestID: req.RequestID,
					OK:        true,
					Code:      "ok",
					Result:    map[string]any{},
				}
				switch req.Op {
				case "webauthn.register.finish":
					credentialID := "cred-test-1"
					if v, ok := req.Payload["credential_id"].(string); ok && v != "" {
						credentialID = v
					}
					resp.Result = map[string]any{
						"credential_id":          credentialID,
						"public_key_cose_b64url": "Y29zZS1wdWJrZXk",
						"sign_count":             1,
					}
				case "webauthn.assertion.finish":
					stored := int64(0)
					if v, ok := req.Payload["stored_sign_count"].(float64); ok {
						stored = int64(v)
					}
					resp.Result = map[string]any{
						"credential_id": req.Payload["credential_id"],
						"sign_count":    stored + 1,
					}
				case "totp.verify":
					code, _ := req.Payload["code"].(string)
					resp.Result = map[string]any{
						"valid":           code == "123456",
						"matched_counter": 100,
					}
				case "crypto.pgp.sign":
					plaintext, _ := req.Payload["plaintext"].(string)
					resp.Result = map[string]any{
						"signed_message_armored": "PGP-SIGNED:" + plaintext,
					}
				case "crypto.pgp.encrypt":
					plaintext, _ := req.Payload["plaintext"].(string)
					resp.Result = map[string]any{
						"ciphertext_armored": "PGP-ENC:" + plaintext,
					}
				case "crypto.pgp.decrypt":
					ciphertext, _ := req.Payload["ciphertext_armored"].(string)
					resp.Result = map[string]any{
						"plaintext_utf8": "PGP-DEC:" + ciphertext,
					}
				case "crypto.pgp.verify":
					resp.Result = map[string]any{
						"valid":          true,
						"plaintext_utf8": "PGP-VERIFIED",
					}
				case "crypto.smime.sign":
					plaintext, _ := req.Payload["plaintext"].(string)
					resp.Result = map[string]any{
						"signed_smime": "SMIME-SIGNED:" + plaintext,
					}
				case "crypto.smime.encrypt":
					plaintext, _ := req.Payload["plaintext"].(string)
					resp.Result = map[string]any{
						"ciphertext_smime": "SMIME-ENC:" + plaintext,
					}
				case "crypto.smime.decrypt":
					ciphertext, _ := req.Payload["ciphertext_smime"].(string)
					resp.Result = map[string]any{
						"plaintext_utf8": "SMIME-DEC:" + ciphertext,
					}
				case "crypto.smime.verify":
					resp.Result = map[string]any{
						"valid":          true,
						"plaintext_utf8": "SMIME-VERIFIED",
					}
				default:
					resp.OK = false
					resp.Code = "unsupported_operation"
					resp.Error = "unsupported op"
				}
				out, err := json.Marshal(resp)
				if err != nil {
					return
				}
				binary.BigEndian.PutUint32(lenBuf, uint32(len(out)))
				_, _ = c.Write(lenBuf)
				_, _ = c.Write(out)
			}(conn)
		}
	}()
	return socketPath, recorder
}

type captureRecoveryMFASender struct {
	captureResetSender
	recoveryCalls int
	recoveryTo    string
	recoveryCode  string
	recoveryErr   error
}

func (s *captureRecoveryMFASender) SendRecoveryEmailCode(ctx context.Context, toEmail, code string) error {
	_ = ctx
	s.recoveryCalls++
	s.recoveryTo = toEmail
	s.recoveryCode = code
	return s.recoveryErr
}

func loginV2WithResponse(t *testing.T, router http.Handler) (*http.Cookie, *http.Cookie, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    "admin@example.com",
		"password": "SecretPass123!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/login", bytes.NewReader(body))
	setTestLoopbackOrigin(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode login response: %v", err)
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
		t.Fatalf("missing session/csrf cookie")
	}
	return sessionCookie, csrfCookie, payload
}

func loginV2(t *testing.T, router http.Handler) (*http.Cookie, *http.Cookie) {
	t.Helper()
	sess, csrf, _ := loginV2WithResponse(t, router)
	return sess, csrf
}

func loginV1WithResponse(t *testing.T, router http.Handler, email, password string) (*http.Cookie, *http.Cookie, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader(body))
	setTestLoopbackOrigin(req)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected v1 login 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode v1 login response: %v", err)
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
		t.Fatalf("missing session/csrf cookie on v1 login")
	}
	return sessionCookie, csrfCookie, payload
}

func doV1AuthedJSON(t *testing.T, router http.Handler, method, path string, payload any, sess, csrf *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	setTestLoopbackOrigin(req)
	req.AddCookie(sess)
	req.AddCookie(csrf)
	if method != http.MethodGet {
		req.Header.Set("X-CSRF-Token", csrf.Value)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func doV2AuthedJSON(t *testing.T, router http.Handler, method, path string, payload any, sess, csrf *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	setTestLoopbackOrigin(req)
	req.AddCookie(sess)
	req.AddCookie(csrf)
	if method != http.MethodGet {
		req.Header.Set("X-CSRF-Token", csrf.Value)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func doV2AuthedMultipart(t *testing.T, router http.Handler, method, path string, build func(*multipart.Writer), sess, csrf *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if build != nil {
		build(writer)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(method, path, &body)
	setTestLoopbackOrigin(req)
	req.AddCookie(sess)
	req.AddCookie(csrf)
	if method != http.MethodGet {
		req.Header.Set("X-CSRF-Token", csrf.Value)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func createV2TestAccount(t *testing.T, router http.Handler, sess, csrf *http.Cookie, login string) models.MailAccount {
	t.Helper()
	rec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts", map[string]any{
		"display_name": "Primary",
		"login":        login,
		"password":     "mailbox-secret",
		"imap_host":    "imap.example.com",
		"imap_port":    993,
		"smtp_host":    "smtp.example.com",
		"smtp_port":    587,
	}, sess, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var account models.MailAccount
	if err := json.Unmarshal(rec.Body.Bytes(), &account); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	return account
}

func createV2TestIdentity(t *testing.T, router http.Handler, sess, csrf *http.Cookie, accountID, fromEmail string) models.MailIdentity {
	t.Helper()
	rec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts/"+accountID+"/identities", map[string]any{
		"display_name": "Primary Alias",
		"from_email":   fromEmail,
		"is_default":   true,
	}, sess, csrf)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected identity create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var identity models.MailIdentity
	if err := json.Unmarshal(rec.Body.Bytes(), &identity); err != nil {
		t.Fatalf("decode identity response: %v", err)
	}
	return identity
}

func setTestLoopbackOrigin(req *http.Request) {
	if req == nil {
		return
	}
	req.Host = "localhost"
	req.URL.Scheme = "http"
	req.URL.Host = "localhost"
}

func extractCredentialIDsFromAllowList(raw any) []string {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := row["id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	return out
}

func TestV2PreferencesFlow(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	rec := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/preferences", nil, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	put := doV2AuthedJSON(t, router, http.MethodPut, "/api/v2/preferences", map[string]any{
		"theme":               "paper-light",
		"page_size":           80,
		"grouping_mode":       "today_yesterday",
		"remote_image_policy": "ask",
		"keymap": map[string]string{
			"compose": "n",
		},
	}, sess, csrf)
	if put.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", put.Code, put.Body.String())
	}

	rec2 := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/preferences", nil, sess, csrf)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec2.Code, rec2.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := out["theme"]; got != "paper-light" {
		t.Fatalf("expected theme=paper-light got=%v", got)
	}
	if got := int(out["page_size"].(float64)); got != 80 {
		t.Fatalf("expected page_size=80 got=%d", got)
	}
}

func TestV2SavedSearchRoundTripPreservesAdvancedFiltersJSON(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)
	filtersJSON := `{"account_scope":"all","view_kind":"mailbox","mailbox":"","query":"alpha","filters":{"from":"alice@example.com","to":"team@example.com","subject":"launch","date_from":"2026-03-10","date_to":"2026-03-11","unread":true,"flagged":true,"has_attachments":true,"waiting":false,"account_ids":["acct-a","acct-b"]}}`

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/saved-searches", map[string]any{
		"account_id":   "",
		"name":         "Alpha view",
		"filters_json": filtersJSON,
		"pinned":       true,
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", create.Code, create.Body.String())
	}

	var saved models.SavedSearch
	if err := json.Unmarshal(create.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode create saved search: %v body=%s", err, create.Body.String())
	}
	if saved.FiltersJSON != filtersJSON {
		t.Fatalf("expected filters_json to roundtrip, got %s", saved.FiltersJSON)
	}

	list := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/saved-searches", nil, sess, csrf)
	if list.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", list.Code, list.Body.String())
	}
	var payload struct {
		Items []models.SavedSearch `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list saved searches: %v body=%s", err, list.Body.String())
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected one saved search, got %d", len(payload.Items))
	}
	if payload.Items[0].FiltersJSON != filtersJSON {
		t.Fatalf("expected saved filters_json to persist, got %s", payload.Items[0].FiltersJSON)
	}
}

func TestV2AccountsCreateAndList(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts", map[string]any{
		"display_name": "Primary",
		"login":        "admin@example.com",
		"password":     "mailbox-secret",
		"imap_host":    "imap.example.com",
		"imap_port":    993,
		"smtp_host":    "smtp.example.com",
		"smtp_port":    587,
		"is_default":   true,
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", create.Code, create.Body.String())
	}

	list := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts", nil, sess, csrf)
	if list.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", list.Code, list.Body.String())
	}
	var out struct {
		Items []models.MailAccount `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("expected 1 account got=%d", len(out.Items))
	}
	if out.Items[0].Login != "admin@example.com" {
		t.Fatalf("unexpected account login: %s", out.Items[0].Login)
	}
}

func TestV2AccountsListBackfillsPAMSessionAccount(t *testing.T) {
	router, st := newV2RouterWithMailClientAndStore(t, pamLoginTestDespatch{
		acceptPassword: "SecretPass123!",
		acceptedUsers:  map[string]bool{"admin": true},
	}, func(cfg *config.Config) {
		cfg.DovecotAuthMode = "pam"
		cfg.IMAPHost = "127.0.0.1"
		cfg.IMAPPort = 993
		cfg.IMAPTLS = true
		cfg.SMTPHost = "127.0.0.1"
		cfg.SMTPPort = 25
		cfg.SMTPTLS = false
		cfg.SMTPStartTLS = false
	})

	sess, csrf, _ := loginV2WithResponse(t, router)
	admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	accounts, err := st.ListMailAccounts(context.Background(), admin.ID)
	if err != nil {
		t.Fatalf("list accounts after login: %v", err)
	}
	for _, account := range accounts {
		if err := st.DeleteMailAccount(context.Background(), admin.ID, account.ID); err != nil {
			t.Fatalf("delete account %s: %v", account.ID, err)
		}
	}

	list := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts", nil, sess, csrf)
	if list.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", list.Code, list.Body.String())
	}
	var out struct {
		Items []models.MailAccount `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("expected 1 bootstrapped account got=%d body=%s", len(out.Items), list.Body.String())
	}
	if out.Items[0].Login != "admin" {
		t.Fatalf("expected bootstrapped login admin, got %q", out.Items[0].Login)
	}
	if !out.Items[0].IsDefault {
		t.Fatalf("expected bootstrapped account to be default")
	}
}

func TestV2TOTPFlowWithMailSecVerifier(t *testing.T) {
	socketPath := startFakeMailSecServer(t)
	router := newV2RouterWithConfig(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
	})
	sess, csrf := loginV2(t, router)

	enroll := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/totp/enroll", map[string]any{}, sess, csrf)
	if enroll.Code != http.StatusOK {
		t.Fatalf("expected enroll 200, got %d body=%s", enroll.Code, enroll.Body.String())
	}
	confirm := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/totp/confirm", map[string]any{
		"code":               "123456",
		"recovery_codes_ack": true,
	}, sess, csrf)
	if confirm.Code != http.StatusOK {
		t.Fatalf("expected confirm 200, got %d body=%s", confirm.Code, confirm.Body.String())
	}

	sess2, csrf2, login := loginV2WithResponse(t, router)
	if required, _ := login["mfa_required"].(bool); !required {
		t.Fatalf("expected mfa_required=true after totp enrollment")
	}

	invalid := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mfa/totp/verify", map[string]any{
		"code": "999999",
	}, sess2, csrf2)
	if invalid.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid verify 401, got %d body=%s", invalid.Code, invalid.Body.String())
	}

	valid := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mfa/totp/verify", map[string]any{
		"code": "123456",
	}, sess2, csrf2)
	if valid.Code != http.StatusOK {
		t.Fatalf("expected valid verify 200, got %d body=%s", valid.Code, valid.Body.String())
	}
}

func TestV2TrustedDeviceRememberAndRevokeFlow(t *testing.T) {
	socketPath := startFakeMailSecServer(t)
	router := newV2RouterWithConfig(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
	})

	sess, csrf := loginV2(t, router)

	enroll := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/totp/enroll", map[string]any{}, sess, csrf)
	if enroll.Code != http.StatusOK {
		t.Fatalf("expected enroll 200, got %d body=%s", enroll.Code, enroll.Body.String())
	}
	confirm := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/totp/confirm", map[string]any{
		"code":               "123456",
		"recovery_codes_ack": true,
	}, sess, csrf)
	if confirm.Code != http.StatusOK {
		t.Fatalf("expected confirm 200, got %d body=%s", confirm.Code, confirm.Body.String())
	}

	sess2, csrf2, login := loginV2WithResponse(t, router)
	if required, _ := login["mfa_required"].(bool); !required {
		t.Fatalf("expected mfa_required=true after enrollment")
	}

	verify := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mfa/totp/verify", map[string]any{
		"code":            "123456",
		"remember_device": true,
	}, sess2, csrf2)
	if verify.Code != http.StatusOK {
		t.Fatalf("expected verify 200, got %d body=%s", verify.Code, verify.Body.String())
	}
	var trustedCookie *http.Cookie
	for _, c := range verify.Result().Cookies() {
		if c.Name == "despatch_mfa_trusted" {
			trustedCookie = c
			break
		}
	}
	if trustedCookie == nil || strings.TrimSpace(trustedCookie.Value) == "" {
		t.Fatalf("expected trusted device cookie after remember_device verify")
	}

	loginBody, _ := json.Marshal(map[string]string{
		"email":    "admin@example.com",
		"password": "SecretPass123!",
	})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v2/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.AddCookie(trustedCookie)
	loginRec := httptest.NewRecorder()
	router.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected trusted-device login 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginPayload map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginPayload); err != nil {
		t.Fatalf("decode trusted-device login payload: %v", err)
	}
	if stage, _ := loginPayload["auth_stage"].(string); stage != "authenticated" {
		t.Fatalf("expected authenticated stage with trusted device, got %v payload=%v", stage, loginPayload)
	}
	if required, _ := loginPayload["mfa_required"].(bool); required {
		t.Fatalf("expected mfa_required=false with trusted device, payload=%v", loginPayload)
	}

	var sess3, csrf3, rotatedTrusted *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		switch c.Name {
		case "despatch_session":
			sess3 = c
		case "despatch_csrf":
			csrf3 = c
		case "despatch_mfa_trusted":
			rotatedTrusted = c
		}
	}
	if sess3 == nil || csrf3 == nil || rotatedTrusted == nil {
		t.Fatalf("expected rotated trusted + session cookies after trusted-device login")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v2/security/mfa/trusted-devices", nil)
	setTestLoopbackOrigin(listReq)
	listReq.AddCookie(sess3)
	listReq.AddCookie(csrf3)
	listReq.AddCookie(rotatedTrusted)
	listTrusted := httptest.NewRecorder()
	router.ServeHTTP(listTrusted, listReq)
	if listTrusted.Code != http.StatusOK {
		t.Fatalf("expected trusted devices list 200, got %d body=%s", listTrusted.Code, listTrusted.Body.String())
	}
	var trustedPayload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(listTrusted.Body.Bytes(), &trustedPayload); err != nil {
		t.Fatalf("decode trusted devices list: %v", err)
	}
	if len(trustedPayload.Items) == 0 {
		t.Fatalf("expected at least one trusted device after remember flow")
	}
	currentFound := false
	for _, item := range trustedPayload.Items {
		if isCurrent, _ := item["is_current"].(bool); isCurrent {
			currentFound = true
		}
		if _, ok := item["display_label"].(string); !ok {
			t.Fatalf("expected display_label in trusted device payload item=%v", item)
		}
		if _, ok := item["browser"].(string); !ok {
			t.Fatalf("expected browser in trusted device payload item=%v", item)
		}
		if _, ok := item["os"].(string); !ok {
			t.Fatalf("expected os in trusted device payload item=%v", item)
		}
		if _, ok := item["device_type"].(string); !ok {
			t.Fatalf("expected device_type in trusted device payload item=%v", item)
		}
	}
	if !currentFound {
		t.Fatalf("expected one trusted device marked is_current=true payload=%v", trustedPayload.Items)
	}

	revokeAll := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/trusted-devices/revoke-all", map[string]any{}, sess3, csrf3)
	if revokeAll.Code != http.StatusOK {
		t.Fatalf("expected revoke-all 200, got %d body=%s", revokeAll.Code, revokeAll.Body.String())
	}

	loginReq2 := httptest.NewRequest(http.MethodPost, "/api/v2/login", bytes.NewReader(loginBody))
	loginReq2.Header.Set("Content-Type", "application/json")
	loginReq2.AddCookie(rotatedTrusted)
	loginRec2 := httptest.NewRecorder()
	router.ServeHTTP(loginRec2, loginReq2)
	if loginRec2.Code != http.StatusOK {
		t.Fatalf("expected login 200 after revoke-all, got %d body=%s", loginRec2.Code, loginRec2.Body.String())
	}
	var loginPayload2 map[string]any
	if err := json.Unmarshal(loginRec2.Body.Bytes(), &loginPayload2); err != nil {
		t.Fatalf("decode login payload after revoke-all: %v", err)
	}
	if stage, _ := loginPayload2["auth_stage"].(string); stage != "mfa_required" {
		t.Fatalf("expected mfa_required stage after trusted devices revoked, got %v payload=%v", stage, loginPayload2)
	}
}

func TestV2MFARecoveryEmailFallbackFlow(t *testing.T) {
	socketPath := startFakeMailSecServer(t)
	sender := &captureRecoveryMFASender{}
	router, st, _ := newV2RouterWithMailClientStoreAndHook(t, mail.NoopClient{}, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
	}, func(svc *service.Service, st *store.Store, _ *sql.DB, _ *config.Config) {
		svc.SetNotificationSender(sender)
	})
	ctx := context.Background()
	admin, err := st.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(ctx, admin.ID, "recovery@example.net"); err != nil {
		t.Fatalf("set recovery email: %v", err)
	}

	sess, csrf := loginV2(t, router)
	enroll := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/totp/enroll", map[string]any{}, sess, csrf)
	if enroll.Code != http.StatusOK {
		t.Fatalf("expected enroll 200, got %d body=%s", enroll.Code, enroll.Body.String())
	}
	confirm := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/totp/confirm", map[string]any{
		"code":               "123456",
		"recovery_codes_ack": true,
	}, sess, csrf)
	if confirm.Code != http.StatusOK {
		t.Fatalf("expected confirm 200, got %d body=%s", confirm.Code, confirm.Body.String())
	}

	pendingSess, pendingCSRF, login := loginV2WithResponse(t, router)
	if required, _ := login["mfa_required"].(bool); !required {
		t.Fatalf("expected mfa_required=true after totp enrollment")
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mfa/recovery-email/send", map[string]any{}, pendingSess, pendingCSRF)
	if send.Code != http.StatusOK {
		t.Fatalf("expected recovery email send 200, got %d body=%s", send.Code, send.Body.String())
	}
	if sender.recoveryCalls != 1 {
		t.Fatalf("expected one recovery email code send, got %d", sender.recoveryCalls)
	}
	if sender.recoveryTo != "recovery@example.net" {
		t.Fatalf("expected delivery to recovery@example.net, got %q", sender.recoveryTo)
	}
	if strings.TrimSpace(sender.recoveryCode) == "" {
		t.Fatalf("expected non-empty recovery email code")
	}

	verify := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mfa/recovery-email/verify", map[string]any{
		"code":            sender.recoveryCode,
		"remember_device": true,
	}, pendingSess, pendingCSRF)
	if verify.Code != http.StatusOK {
		t.Fatalf("expected recovery email verify 200, got %d body=%s", verify.Code, verify.Body.String())
	}
	trustedCookieFound := false
	for _, c := range verify.Result().Cookies() {
		if c.Name == "despatch_mfa_trusted" && strings.TrimSpace(c.Value) != "" {
			trustedCookieFound = true
			break
		}
	}
	if !trustedCookieFound {
		t.Fatalf("expected trusted device cookie after recovery email verification")
	}

	unblocked := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts", nil, pendingSess, pendingCSRF)
	if unblocked.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200 after recovery email verification, got %d body=%s", unblocked.Code, unblocked.Body.String())
	}
}

func TestV2MFADeviceApprovalFlow(t *testing.T) {
	socketPath := startFakeMailSecServer(t)
	router := newV2RouterWithConfig(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
	})

	approverSess, approverCSRF := loginV2(t, router)
	enroll := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/totp/enroll", map[string]any{}, approverSess, approverCSRF)
	if enroll.Code != http.StatusOK {
		t.Fatalf("expected enroll 200, got %d body=%s", enroll.Code, enroll.Body.String())
	}
	confirm := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/totp/confirm", map[string]any{
		"code":               "123456",
		"recovery_codes_ack": true,
	}, approverSess, approverCSRF)
	if confirm.Code != http.StatusOK {
		t.Fatalf("expected confirm 200, got %d body=%s", confirm.Code, confirm.Body.String())
	}

	pendingSess, pendingCSRF, login := loginV2WithResponse(t, router)
	if required, _ := login["mfa_required"].(bool); !required {
		t.Fatalf("expected mfa_required=true after totp enrollment")
	}

	request := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mfa/device-approval/request", map[string]any{
		"remember_device": true,
	}, pendingSess, pendingCSRF)
	if request.Code != http.StatusOK {
		t.Fatalf("expected device approval request 200, got %d body=%s", request.Code, request.Body.String())
	}
	var requestPayload map[string]any
	if err := json.Unmarshal(request.Body.Bytes(), &requestPayload); err != nil {
		t.Fatalf("decode device approval request payload: %v", err)
	}
	requestID, _ := requestPayload["request_id"].(string)
	if strings.TrimSpace(requestID) == "" {
		t.Fatalf("expected non-empty request_id payload=%v", requestPayload)
	}
	if count, _ := requestPayload["other_device_count"].(float64); count < 1 {
		t.Fatalf("expected at least one eligible device, payload=%v", requestPayload)
	}

	pending := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/security/mfa/device-approvals/pending", nil, approverSess, approverCSRF)
	if pending.Code != http.StatusOK {
		t.Fatalf("expected pending approvals 200, got %d body=%s", pending.Code, pending.Body.String())
	}
	var pendingPayload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(pending.Body.Bytes(), &pendingPayload); err != nil {
		t.Fatalf("decode pending approvals payload: %v", err)
	}
	if len(pendingPayload.Items) == 0 {
		t.Fatalf("expected at least one pending approval request")
	}
	if got, _ := pendingPayload.Items[0]["id"].(string); got != requestID {
		t.Fatalf("expected pending approval id %q, got %q", requestID, got)
	}

	decision := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/device-approvals/"+requestID+"/decision", map[string]any{
		"decision": "approve",
	}, approverSess, approverCSRF)
	if decision.Code != http.StatusOK {
		t.Fatalf("expected approval decision 200, got %d body=%s", decision.Code, decision.Body.String())
	}

	status := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/mfa/device-approval/status?request_id="+url.QueryEscape(requestID), nil, pendingSess, pendingCSRF)
	if status.Code != http.StatusOK {
		t.Fatalf("expected approval status 200, got %d body=%s", status.Code, status.Body.String())
	}
	var statusPayload map[string]any
	if err := json.Unmarshal(status.Body.Bytes(), &statusPayload); err != nil {
		t.Fatalf("decode approval status payload: %v", err)
	}
	if got, _ := statusPayload["status"].(string); got != "approved" {
		t.Fatalf("expected approved status payload=%v", statusPayload)
	}
	trustedCookieFound := false
	for _, c := range status.Result().Cookies() {
		if c.Name == "despatch_mfa_trusted" && strings.TrimSpace(c.Value) != "" {
			trustedCookieFound = true
			break
		}
	}
	if !trustedCookieFound {
		t.Fatalf("expected trusted device cookie after device approval verification")
	}

	unblocked := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts", nil, pendingSess, pendingCSRF)
	if unblocked.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200 after device approval, got %d body=%s", unblocked.Code, unblocked.Body.String())
	}
}

func TestV2SessionsListMarksCurrentSession(t *testing.T) {
	router := newV2Router(t)
	sess, csrf, _ := loginV2WithResponse(t, router)

	rec := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/security/sessions", nil, sess, csrf)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected sessions list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode sessions payload: %v", err)
	}
	if len(payload.Items) == 0 {
		t.Fatalf("expected at least one session in list")
	}
	currentFound := false
	for _, item := range payload.Items {
		if isCurrent, _ := item["is_current"].(bool); isCurrent {
			currentFound = true
		}
	}
	if !currentFound {
		t.Fatalf("expected is_current=true in sessions payload=%v", payload.Items)
	}
}

func TestV2WebAuthnRegistrationAndMFAGate(t *testing.T) {
	socketPath := startFakeMailSecServer(t)
	router := newV2RouterWithConfig(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
	})
	sess, csrf, login := loginV2WithResponse(t, router)
	if required, _ := login["mfa_required"].(bool); required {
		t.Fatalf("expected mfa_required=false before enrollment")
	}

	beginReg := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/webauthn/register/begin", map[string]any{}, sess, csrf)
	if beginReg.Code != http.StatusOK {
		t.Fatalf("expected register begin 200, got %d body=%s", beginReg.Code, beginReg.Body.String())
	}
	var beginRegPayload map[string]any
	if err := json.Unmarshal(beginReg.Body.Bytes(), &beginRegPayload); err != nil {
		t.Fatalf("decode register begin: %v", err)
	}
	challenge, _ := beginRegPayload["challenge"].(string)
	if challenge == "" {
		t.Fatalf("expected non-empty challenge")
	}

	finishReg := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/webauthn/register/finish", map[string]any{
		"challenge":  challenge,
		"id":         "cred-test-1",
		"rawId":      "cred-test-1",
		"transports": []string{"internal"},
		"name":       "Laptop Passkey",
		"response": map[string]any{
			"clientDataJSON":    "Y2xpZW50LWRhdGE",
			"attestationObject": "YXR0ZXN0YXRpb24tb2JqZWN0",
		},
	}, sess, csrf)
	if finishReg.Code != http.StatusCreated {
		t.Fatalf("expected register finish 201, got %d body=%s", finishReg.Code, finishReg.Body.String())
	}
	var finishPayload map[string]any
	if err := json.Unmarshal(finishReg.Body.Bytes(), &finishPayload); err != nil {
		t.Fatalf("decode register finish: %v", err)
	}
	credential, ok := finishPayload["credential"].(map[string]any)
	if !ok {
		t.Fatalf("expected credential object in register finish payload: %v", finishPayload)
	}
	credentialID, _ := credential["id"].(string)
	if strings.TrimSpace(credentialID) == "" {
		t.Fatalf("expected created credential id")
	}

	sess2, csrf2, login2 := loginV2WithResponse(t, router)
	if required, _ := login2["mfa_required"].(bool); !required {
		t.Fatalf("expected mfa_required=true after enrollment")
	}

	blocked := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts", nil, sess2, csrf2)
	if blocked.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 before mfa verify, got %d body=%s", blocked.Code, blocked.Body.String())
	}

	beginLogin := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mfa/webauthn/begin", map[string]any{}, sess2, csrf2)
	if beginLogin.Code != http.StatusOK {
		t.Fatalf("expected login begin 200, got %d body=%s", beginLogin.Code, beginLogin.Body.String())
	}
	var beginLoginPayload map[string]any
	if err := json.Unmarshal(beginLogin.Body.Bytes(), &beginLoginPayload); err != nil {
		t.Fatalf("decode login begin: %v", err)
	}
	loginChallenge, _ := beginLoginPayload["challenge"].(string)
	if loginChallenge == "" {
		t.Fatalf("expected non-empty login challenge")
	}

	finishLogin := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mfa/webauthn/finish", map[string]any{
		"id":    "cred-test-1",
		"rawId": "cred-test-1",
		"response": map[string]any{
			"clientDataJSON":    "Y2xpZW50LWRhdGE",
			"authenticatorData": "YXV0aGVudGljYXRvci1kYXRh",
			"signature":         "c2lnbmF0dXJl",
		},
	}, sess2, csrf2)
	if finishLogin.Code != http.StatusOK {
		t.Fatalf("expected login finish 200, got %d body=%s", finishLogin.Code, finishLogin.Body.String())
	}

	unblocked := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts", nil, sess2, csrf2)
	if unblocked.Code != http.StatusOK {
		t.Fatalf("expected 200 after mfa verify, got %d body=%s", unblocked.Code, unblocked.Body.String())
	}

	del := doV2AuthedJSON(t, router, http.MethodDelete, "/api/v2/security/mfa/webauthn/"+credentialID, nil, sess, csrf)
	if del.Code != http.StatusOK {
		t.Fatalf("expected credential delete 200, got %d body=%s", del.Code, del.Body.String())
	}
}

func TestV2PasskeyLoginAndMailSecretUnlockFlow(t *testing.T) {
	socketPath := startFakeMailSecServer(t)
	router, st := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
		cfg.PasskeyPasswordlessEnabled = true
		cfg.PasskeyUsernamelessEnabled = true
	})
	admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	if _, err := st.UpsertMFAWebAuthnCredential(context.Background(), models.MFAWebAuthnCredential{
		UserID:         admin.ID,
		CredentialID:   "cred-passkey-primary",
		PublicKey:      "Y29zZS1wdWJrZXk",
		SignCount:      1,
		TransportsJSON: `["internal"]`,
		Name:           "Primary Passkey",
	}); err != nil {
		t.Fatalf("seed passkey credential: %v", err)
	}

	beginReq := httptest.NewRequest(http.MethodPost, "/api/v2/login/passkey/begin", bytes.NewReader([]byte(`{}`)))
	setTestLoopbackOrigin(beginReq)
	beginReq.Header.Set("Content-Type", "application/json")
	beginRec := httptest.NewRecorder()
	router.ServeHTTP(beginRec, beginReq)
	if beginRec.Code != http.StatusOK {
		t.Fatalf("expected passkey begin 200, got %d body=%s", beginRec.Code, beginRec.Body.String())
	}
	var beginPayload map[string]any
	if err := json.Unmarshal(beginRec.Body.Bytes(), &beginPayload); err != nil {
		t.Fatalf("decode passkey begin payload: %v", err)
	}
	challengeID, _ := beginPayload["challenge_id"].(string)
	challenge, _ := beginPayload["challenge"].(string)
	if strings.TrimSpace(challengeID) == "" || strings.TrimSpace(challenge) == "" {
		t.Fatalf("expected challenge_id and challenge in begin payload: %v", beginPayload)
	}
	var challengeCookie *http.Cookie
	for _, c := range beginRec.Result().Cookies() {
		if c.Name == "despatch_passkey_challenge" {
			challengeCookie = c
			break
		}
	}
	if challengeCookie == nil {
		t.Fatalf("expected passkey challenge cookie to be set")
	}

	finishBody, _ := json.Marshal(map[string]any{
		"challenge_id": challengeID,
		"challenge":    challenge,
		"id":           "cred-passkey-primary",
		"rawId":        "cred-passkey-primary",
		"response": map[string]any{
			"clientDataJSON":    "Y2xpZW50LWRhdGE",
			"authenticatorData": "YXV0aGVudGljYXRvci1kYXRh",
			"signature":         "c2lnbmF0dXJl",
		},
	})
	finishReq := httptest.NewRequest(http.MethodPost, "/api/v2/login/passkey/finish", bytes.NewReader(finishBody))
	setTestLoopbackOrigin(finishReq)
	finishReq.Header.Set("Content-Type", "application/json")
	finishReq.AddCookie(challengeCookie)
	finishRec := httptest.NewRecorder()
	router.ServeHTTP(finishRec, finishReq)
	if finishRec.Code != http.StatusOK {
		t.Fatalf("expected passkey finish 200, got %d body=%s", finishRec.Code, finishRec.Body.String())
	}
	var finishPayload map[string]any
	if err := json.Unmarshal(finishRec.Body.Bytes(), &finishPayload); err != nil {
		t.Fatalf("decode passkey finish payload: %v", err)
	}
	if stage, _ := finishPayload["auth_stage"].(string); stage != "authenticated" {
		t.Fatalf("expected authenticated stage after passkey login, got %q payload=%v", stage, finishPayload)
	}
	if required, _ := finishPayload["mail_secret_required"].(bool); !required {
		t.Fatalf("expected mail_secret_required=true for first passkey login without stored secret")
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/api/v2/login/passkey/finish", bytes.NewReader(finishBody))
	setTestLoopbackOrigin(replayReq)
	replayReq.Header.Set("Content-Type", "application/json")
	replayReq.AddCookie(challengeCookie)
	replayRec := httptest.NewRecorder()
	router.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected replayed passkey finish 401, got %d body=%s", replayRec.Code, replayRec.Body.String())
	}
	var replayPayload map[string]any
	if err := json.Unmarshal(replayRec.Body.Bytes(), &replayPayload); err != nil {
		t.Fatalf("decode replay payload: %v", err)
	}
	if got, _ := replayPayload["code"].(string); got != "webauthn_challenge_invalid" {
		t.Fatalf("expected webauthn_challenge_invalid on replay, got %q payload=%v", got, replayPayload)
	}

	var sess, csrf *http.Cookie
	for _, c := range finishRec.Result().Cookies() {
		switch c.Name {
		case "despatch_session":
			sess = c
		case "despatch_csrf":
			csrf = c
		}
	}
	if sess == nil || csrf == nil {
		t.Fatalf("expected session/csrf cookies on passkey finish")
	}

	mailBeforeUnlock := doV1AuthedJSON(t, router, http.MethodGet, "/api/v1/mailboxes", nil, sess, csrf)
	if mailBeforeUnlock.Code != http.StatusUnauthorized {
		t.Fatalf("expected mailbox request 401 before unlock, got %d body=%s", mailBeforeUnlock.Code, mailBeforeUnlock.Body.String())
	}

	unlock := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/session/mail-secret/unlock", map[string]any{
		"password": "SecretPass123!",
	}, sess, csrf)
	if unlock.Code != http.StatusOK {
		t.Fatalf("expected mail-secret unlock 200, got %d body=%s", unlock.Code, unlock.Body.String())
	}

	mailAfterUnlock := doV1AuthedJSON(t, router, http.MethodGet, "/api/v1/mailboxes", nil, sess, csrf)
	if mailAfterUnlock.Code != http.StatusOK {
		t.Fatalf("expected mailbox request 200 after unlock, got %d body=%s", mailAfterUnlock.Code, mailAfterUnlock.Body.String())
	}
}

func TestV2PasskeyLoginAcceptsLegacyHexCredentialIDs(t *testing.T) {
	socketPath := startFakeMailSecServer(t)
	router, st := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
		cfg.PasskeyPasswordlessEnabled = true
		cfg.PasskeyUsernamelessEnabled = true
	})
	admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	credentialBytes := []byte("legacy-passkey-cred")
	credentialIDHex := hex.EncodeToString(credentialBytes)
	credentialIDB64 := base64.RawURLEncoding.EncodeToString(credentialBytes)
	if _, err := st.UpsertMFAWebAuthnCredential(context.Background(), models.MFAWebAuthnCredential{
		UserID:         admin.ID,
		CredentialID:   credentialIDHex,
		PublicKey:      "Y29zZS1wdWJrZXk",
		SignCount:      1,
		TransportsJSON: `["internal"]`,
		Name:           "Legacy Hex Passkey",
	}); err != nil {
		t.Fatalf("seed passkey credential: %v", err)
	}

	beginReq := httptest.NewRequest(http.MethodPost, "/api/v2/login/passkey/begin", bytes.NewReader([]byte(`{"email":"admin@example.com"}`)))
	setTestLoopbackOrigin(beginReq)
	beginReq.Header.Set("Content-Type", "application/json")
	beginRec := httptest.NewRecorder()
	router.ServeHTTP(beginRec, beginReq)
	if beginRec.Code != http.StatusOK {
		t.Fatalf("expected passkey begin 200, got %d body=%s", beginRec.Code, beginRec.Body.String())
	}
	var beginPayload map[string]any
	if err := json.Unmarshal(beginRec.Body.Bytes(), &beginPayload); err != nil {
		t.Fatalf("decode passkey begin payload: %v", err)
	}
	allowList := extractCredentialIDsFromAllowList(beginPayload["allow_credentials"])
	if len(allowList) != 0 {
		t.Fatalf("expected discoverable challenge without allow_credentials filter, got %v", allowList)
	}
	challengeID, _ := beginPayload["challenge_id"].(string)
	challenge, _ := beginPayload["challenge"].(string)
	if strings.TrimSpace(challengeID) == "" || strings.TrimSpace(challenge) == "" {
		t.Fatalf("expected challenge_id and challenge in begin payload: %v", beginPayload)
	}
	var challengeCookie *http.Cookie
	for _, c := range beginRec.Result().Cookies() {
		if c.Name == "despatch_passkey_challenge" {
			challengeCookie = c
			break
		}
	}
	if challengeCookie == nil {
		t.Fatalf("expected passkey challenge cookie to be set")
	}

	finishBody, _ := json.Marshal(map[string]any{
		"challenge_id": challengeID,
		"challenge":    challenge,
		"id":           credentialIDB64,
		"rawId":        credentialIDB64,
		"response": map[string]any{
			"clientDataJSON":    "Y2xpZW50LWRhdGE",
			"authenticatorData": "YXV0aGVudGljYXRvci1kYXRh",
			"signature":         "c2lnbmF0dXJl",
		},
	})
	finishReq := httptest.NewRequest(http.MethodPost, "/api/v2/login/passkey/finish", bytes.NewReader(finishBody))
	setTestLoopbackOrigin(finishReq)
	finishReq.Header.Set("Content-Type", "application/json")
	finishReq.AddCookie(challengeCookie)
	finishRec := httptest.NewRecorder()
	router.ServeHTTP(finishRec, finishReq)
	if finishRec.Code != http.StatusOK {
		t.Fatalf("expected passkey finish 200 for legacy hex credential, got %d body=%s", finishRec.Code, finishRec.Body.String())
	}
}

func TestV2MFAWebAuthnFinishAcceptsLegacyHexCredentialIDs(t *testing.T) {
	socketPath := startFakeMailSecServer(t)
	router, st := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
	})
	sess, csrf := loginV2(t, router)
	admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	credentialBytes := []byte("legacy-mfa-passkey")
	credentialIDHex := hex.EncodeToString(credentialBytes)
	credentialIDB64 := base64.RawURLEncoding.EncodeToString(credentialBytes)
	if _, err := st.UpsertMFAWebAuthnCredential(context.Background(), models.MFAWebAuthnCredential{
		UserID:         admin.ID,
		CredentialID:   credentialIDHex,
		PublicKey:      "Y29zZS1wdWJrZXk",
		SignCount:      1,
		TransportsJSON: `["internal"]`,
		Name:           "Legacy Hex MFA Passkey",
	}); err != nil {
		t.Fatalf("seed passkey credential: %v", err)
	}

	begin := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mfa/webauthn/begin", map[string]any{}, sess, csrf)
	if begin.Code != http.StatusOK {
		t.Fatalf("expected mfa webauthn begin 200, got %d body=%s", begin.Code, begin.Body.String())
	}
	var beginPayload map[string]any
	if err := json.Unmarshal(begin.Body.Bytes(), &beginPayload); err != nil {
		t.Fatalf("decode begin payload: %v", err)
	}
	challenge, _ := beginPayload["challenge"].(string)
	if strings.TrimSpace(challenge) == "" {
		t.Fatalf("expected non-empty challenge")
	}
	allowList := extractCredentialIDsFromAllowList(beginPayload["allow_credentials"])
	if len(allowList) == 0 {
		t.Fatalf("expected allow_credentials to include seeded credential")
	}
	if allowList[0] != credentialIDB64 {
		t.Fatalf("expected allow credential id %q, got %q", credentialIDB64, allowList[0])
	}

	finish := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mfa/webauthn/finish", map[string]any{
		"challenge": challenge,
		"id":        credentialIDB64,
		"rawId":     credentialIDB64,
		"response": map[string]any{
			"clientDataJSON":    "Y2xpZW50LWRhdGE",
			"authenticatorData": "YXV0aGVudGljYXRvci1kYXRh",
			"signature":         "c2lnbmF0dXJl",
		},
	}, sess, csrf)
	if finish.Code != http.StatusOK {
		t.Fatalf("expected mfa webauthn finish 200 for legacy hex credential, got %d body=%s", finish.Code, finish.Body.String())
	}
}

func TestV2PasskeyBeginRejectsInsecureOrigin(t *testing.T) {
	socketPath := startFakeMailSecServer(t)
	router := newV2RouterWithConfig(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
		cfg.PasskeyPasswordlessEnabled = true
		cfg.PasskeyUsernamelessEnabled = true
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/login/passkey/begin", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected insecure-origin reject 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if got, _ := payload["code"].(string); got != "webauthn_insecure_origin" {
		t.Fatalf("expected webauthn_insecure_origin code, got %q payload=%v", got, payload)
	}
}

func TestMFASetupRequiredBlocksV1AndV2UntilEnrollment(t *testing.T) {
	socketPath := startFakeMailSecServer(t)
	router, st := newV2RouterWithConfigAndStore(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
	})

	ctx := context.Background()
	admin, err := st.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	if err := st.UpdateUserMFAPreference(ctx, admin.ID, "totp"); err != nil {
		t.Fatalf("set mfa preference: %v", err)
	}

	sess, csrf, login := loginV1WithResponse(t, router, "admin@example.com", "SecretPass123!")
	if stage, _ := login["auth_stage"].(string); stage != "mfa_setup_required" {
		t.Fatalf("expected auth_stage=mfa_setup_required, got=%v payload=%v", stage, login)
	}
	if setupMethod, _ := login["mfa_setup_method"].(string); setupMethod != "totp" {
		t.Fatalf("expected mfa_setup_method=totp, got=%v payload=%v", setupMethod, login)
	}

	v1Blocked := doV1AuthedJSON(t, router, http.MethodGet, "/api/v1/mailboxes", nil, sess, csrf)
	if v1Blocked.Code != http.StatusUnauthorized {
		t.Fatalf("expected v1 401 before setup, got %d body=%s", v1Blocked.Code, v1Blocked.Body.String())
	}
	var v1Err map[string]any
	_ = json.Unmarshal(v1Blocked.Body.Bytes(), &v1Err)
	if code, _ := v1Err["code"].(string); code != "mfa_setup_required" {
		t.Fatalf("expected v1 mfa_setup_required, got=%v body=%s", code, v1Blocked.Body.String())
	}

	v2Blocked := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts", nil, sess, csrf)
	if v2Blocked.Code != http.StatusUnauthorized {
		t.Fatalf("expected v2 401 before setup, got %d body=%s", v2Blocked.Code, v2Blocked.Body.String())
	}
	var v2Err map[string]any
	_ = json.Unmarshal(v2Blocked.Body.Bytes(), &v2Err)
	if code, _ := v2Err["code"].(string); code != "mfa_setup_required" {
		t.Fatalf("expected v2 mfa_setup_required, got=%v body=%s", code, v2Blocked.Body.String())
	}

	enroll := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/totp/enroll", map[string]any{}, sess, csrf)
	if enroll.Code != http.StatusOK {
		t.Fatalf("expected totp enroll 200, got %d body=%s", enroll.Code, enroll.Body.String())
	}
	confirm := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/totp/confirm", map[string]any{
		"code":               "123456",
		"recovery_codes_ack": true,
	}, sess, csrf)
	if confirm.Code != http.StatusOK {
		t.Fatalf("expected totp confirm 200, got %d body=%s", confirm.Code, confirm.Body.String())
	}

	v2After := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts", nil, sess, csrf)
	if v2After.Code != http.StatusOK {
		t.Fatalf("expected v2 200 after setup, got %d body=%s", v2After.Code, v2After.Body.String())
	}
	v1After := doV1AuthedJSON(t, router, http.MethodGet, "/api/v1/mailboxes", nil, sess, csrf)
	if v1After.Code != http.StatusOK {
		t.Fatalf("expected v1 200 after setup, got %d body=%s", v1After.Code, v1After.Body.String())
	}
}

func TestRegisterPersistsMFAPreference(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	body, _ := json.Marshal(map[string]any{
		"email":          "newuser@example.com",
		"recovery_email": "newuser-recovery@example.net",
		"password":       "StrongPass123!",
		"mfa_preference": "totp",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected register 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	u, err := st.GetUserByEmail(context.Background(), "newuser@example.com")
	if err != nil {
		t.Fatalf("load registered user: %v", err)
	}
	if u.MFAPreference != "totp" {
		t.Fatalf("expected user mfa_preference=totp got=%q", u.MFAPreference)
	}

	items, _, err := st.ListRegistrations(context.Background(), models.RegistrationQuery{
		Status: "pending",
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	found := false
	for _, item := range items {
		if strings.EqualFold(item.Email, "newuser@example.com") {
			found = true
			if item.MFAPreference != "totp" {
				t.Fatalf("expected registration mfa_preference=totp got=%q", item.MFAPreference)
			}
		}
	}
	if !found {
		t.Fatalf("expected pending registration for newuser@example.com")
	}
}

func TestRegisterRequiresRecoveryEmail(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, nil)
	body, _ := json.Marshal(map[string]any{
		"email":          "norecovery@example.com",
		"password":       "StrongPass123!",
		"mfa_preference": "none",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected register 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode register payload: %v body=%s", err, rec.Body.String())
	}
	if payload["code"] != "recovery_email_required" {
		t.Fatalf("expected recovery_email_required, got=%v body=%s", payload["code"], rec.Body.String())
	}
}

func TestRegisterRejectsRecoveryEmailMatchingLogin(t *testing.T) {
	router, _ := newV2RouterWithConfigAndStore(t, nil)
	body, _ := json.Marshal(map[string]any{
		"email":          "same-recovery@example.com",
		"recovery_email": "same-recovery@example.com",
		"password":       "StrongPass123!",
		"mfa_preference": "none",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected register 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode register payload: %v body=%s", err, rec.Body.String())
	}
	if payload["code"] != "recovery_email_matches_login" {
		t.Fatalf("expected recovery_email_matches_login, got=%v body=%s", payload["code"], rec.Body.String())
	}
}

func TestMFASetupPreferenceSwitchAllowsOnlyOneChange(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)

	ctx := context.Background()
	admin, err := st.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	if err := st.UpdateUserMFAPreference(ctx, admin.ID, "webauthn"); err != nil {
		t.Fatalf("set mfa preference: %v", err)
	}

	sess, csrf, login := loginV2WithResponse(t, router)
	if stage, _ := login["auth_stage"].(string); stage != "mfa_setup_required" {
		t.Fatalf("expected auth_stage=mfa_setup_required, got=%v payload=%v", stage, login)
	}

	switchOne := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/preference", map[string]any{
		"preference": "totp",
	}, sess, csrf)
	if switchOne.Code != http.StatusOK {
		t.Fatalf("expected first switch 200, got %d body=%s", switchOne.Code, switchOne.Body.String())
	}

	switchTwo := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/preference", map[string]any{
		"preference": "webauthn",
	}, sess, csrf)
	if switchTwo.Code != http.StatusConflict {
		t.Fatalf("expected second switch 409, got %d body=%s", switchTwo.Code, switchTwo.Body.String())
	}
	var errPayload map[string]any
	_ = json.Unmarshal(switchTwo.Body.Bytes(), &errPayload)
	if code, _ := errPayload["code"].(string); code != "mfa_preference_switch_exhausted" {
		t.Fatalf("expected mfa_preference_switch_exhausted, got=%v body=%s", code, switchTwo.Body.String())
	}
}

func TestLegacyMFAPromptDismissFlow(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	ctx := context.Background()
	admin, err := st.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	if err := st.SetLegacyMFAPromptPending(ctx, admin.ID, true); err != nil {
		t.Fatalf("set legacy prompt: %v", err)
	}
	if err := st.UpdateUserMFAPreference(ctx, admin.ID, "none"); err != nil {
		t.Fatalf("set mfa preference: %v", err)
	}

	sess, csrf, login := loginV1WithResponse(t, router, "admin@example.com", "SecretPass123!")
	if stage, _ := login["auth_stage"].(string); stage != "authenticated" {
		t.Fatalf("expected authenticated stage, got=%v payload=%v", stage, login)
	}
	if prompt, _ := login["legacy_mfa_prompt"].(bool); !prompt {
		t.Fatalf("expected legacy_mfa_prompt=true on login, payload=%v", login)
	}

	dismiss := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/mfa/legacy-dismiss", map[string]any{}, sess, csrf)
	if dismiss.Code != http.StatusOK {
		t.Fatalf("expected legacy dismiss 200, got %d body=%s", dismiss.Code, dismiss.Body.String())
	}

	me := doV1AuthedJSON(t, router, http.MethodGet, "/api/v1/me", nil, sess, csrf)
	if me.Code != http.StatusOK {
		t.Fatalf("expected me 200, got %d body=%s", me.Code, me.Body.String())
	}
	var mePayload map[string]any
	if err := json.Unmarshal(me.Body.Bytes(), &mePayload); err != nil {
		t.Fatalf("decode me payload: %v", err)
	}
	if sessionID, _ := mePayload["session_id"].(string); strings.TrimSpace(sessionID) == "" {
		t.Fatalf("expected non-empty session_id in /api/v1/me payload=%v", mePayload)
	}
	if prompt, _ := mePayload["legacy_mfa_prompt"].(bool); prompt {
		t.Fatalf("expected legacy_mfa_prompt=false after dismiss, payload=%v", mePayload)
	}
}

func TestAdminMFAIsMandatoryWhenEnforcementEnabled(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	ctx := context.Background()
	if err := st.UpsertSetting(ctx, "enforce_admin_mfa", "1"); err != nil {
		t.Fatalf("set enforce_admin_mfa: %v", err)
	}

	sess, csrf, login := loginV2WithResponse(t, router)
	if stage, _ := login["auth_stage"].(string); stage != "mfa_setup_required" {
		t.Fatalf("expected admin auth_stage=mfa_setup_required, got=%v payload=%v", stage, login)
	}
	if setupMethod, _ := login["mfa_setup_method"].(string); setupMethod != "totp" {
		t.Fatalf("expected admin setup method=totp, got=%v payload=%v", setupMethod, login)
	}

	blocked := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts", nil, sess, csrf)
	if blocked.Code != http.StatusUnauthorized {
		t.Fatalf("expected blocked accounts 401, got %d body=%s", blocked.Code, blocked.Body.String())
	}
	var errPayload map[string]any
	if err := json.Unmarshal(blocked.Body.Bytes(), &errPayload); err != nil {
		t.Fatalf("decode blocked response: %v", err)
	}
	if code, _ := errPayload["code"].(string); code != "mfa_setup_required" {
		t.Fatalf("expected code=mfa_setup_required got=%v payload=%v", code, errPayload)
	}
}

func TestRegularUserMFAIsOptionalByDefault(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	ctx := context.Background()
	pwHash, err := auth.HashPassword("UserSecret123!")
	if err != nil {
		t.Fatalf("hash user password: %v", err)
	}
	if _, err := st.CreateUserWithMFA(ctx, "user@example.com", pwHash, "user", models.UserActive, "none"); err != nil {
		t.Fatalf("create user: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"email":    "user@example.com",
		"password": "UserSecret123!",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected user login 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode user login payload: %v", err)
	}
	if stage, _ := payload["auth_stage"].(string); stage != "authenticated" {
		t.Fatalf("expected user auth_stage=authenticated, got=%v payload=%v", stage, payload)
	}
}

func TestV2SessionMailProfilePersistsDisplayNameReplyToAndSignature(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	get := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/mail/session-profile", nil, sess, csrf)
	if get.Code != http.StatusOK {
		t.Fatalf("expected session profile get 200, got %d body=%s", get.Code, get.Body.String())
	}
	var initial models.SessionMailProfile
	if err := json.Unmarshal(get.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial session profile: %v", err)
	}
	if initial.FromEmail != "admin@example.com" {
		t.Fatalf("expected session profile from_email admin@example.com, got %+v", initial)
	}

	update := doV2AuthedJSON(t, router, http.MethodPatch, "/api/v2/mail/session-profile", map[string]any{
		"from_email":     "spoof@example.net",
		"display_name":   "Admin Sender",
		"reply_to":       "reply@example.com",
		"signature_html": "<p>Regards</p>",
		"signature_text": "",
	}, sess, csrf)
	if update.Code != http.StatusOK {
		t.Fatalf("expected session profile patch 200, got %d body=%s", update.Code, update.Body.String())
	}
	var saved models.SessionMailProfile
	if err := json.Unmarshal(update.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode saved session profile: %v", err)
	}
	if saved.FromEmail != "admin@example.com" {
		t.Fatalf("expected from_email to remain authenticated session address, got %+v", saved)
	}
	if saved.DisplayName != "Admin Sender" || saved.ReplyTo != "reply@example.com" {
		t.Fatalf("unexpected saved session profile: %+v", saved)
	}
	if saved.SignatureHTML != "<p>Regards</p>" {
		t.Fatalf("expected signature_html to persist, got %+v", saved)
	}
	if saved.SignatureText != "-- \nRegards" {
		t.Fatalf("expected generated signature_text, got %q", saved.SignatureText)
	}

	getAgain := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/mail/session-profile", nil, sess, csrf)
	if getAgain.Code != http.StatusOK {
		t.Fatalf("expected session profile re-get 200, got %d body=%s", getAgain.Code, getAgain.Body.String())
	}
	var fetched models.SessionMailProfile
	if err := json.Unmarshal(getAgain.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode fetched session profile: %v", err)
	}
	if fetched.DisplayName != saved.DisplayName || fetched.ReplyTo != saved.ReplyTo || fetched.SignatureText != saved.SignatureText {
		t.Fatalf("expected session profile fields to round-trip, got %+v", fetched)
	}
}

func TestComposeIdentitiesIncludesSessionProfileAndAccountIdentityMetadata(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	updateProfile := doV2AuthedJSON(t, router, http.MethodPatch, "/api/v2/mail/session-profile", map[string]any{
		"display_name":   "Admin Session",
		"reply_to":       "session-reply@example.com",
		"signature_html": "<p>Session Signature</p>",
	}, sess, csrf)
	if updateProfile.Code != http.StatusOK {
		t.Fatalf("expected session profile patch 200, got %d body=%s", updateProfile.Code, updateProfile.Body.String())
	}

	createAccount := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts", map[string]any{
		"display_name": "Primary Mail",
		"login":        "mailbox@example.com",
		"password":     "mailbox-secret",
		"imap_host":    "imap.example.com",
		"imap_port":    993,
		"smtp_host":    "smtp.example.com",
		"smtp_port":    587,
	}, sess, csrf)
	if createAccount.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", createAccount.Code, createAccount.Body.String())
	}
	var account models.MailAccount
	if err := json.Unmarshal(createAccount.Body.Bytes(), &account); err != nil {
		t.Fatalf("decode account response: %v", err)
	}

	createIdentity := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts/"+account.ID+"/identities", map[string]any{
		"display_name":   "Primary Alias",
		"from_email":     "alias@example.com",
		"reply_to":       "alias-reply@example.com",
		"signature_html": "<p>Account Signature</p>",
		"is_default":     true,
	}, sess, csrf)
	if createIdentity.Code != http.StatusCreated {
		t.Fatalf("expected identity create 201, got %d body=%s", createIdentity.Code, createIdentity.Body.String())
	}
	var identity models.MailIdentity
	if err := json.Unmarshal(createIdentity.Body.Bytes(), &identity); err != nil {
		t.Fatalf("decode identity response: %v", err)
	}

	compose := doV1AuthedJSON(t, router, http.MethodGet, "/api/v1/compose/identities", nil, sess, csrf)
	if compose.Code != http.StatusOK {
		t.Fatalf("expected compose identities 200, got %d body=%s", compose.Code, compose.Body.String())
	}
	var payload struct {
		AuthEmail string `json:"auth_email"`
		Items     []struct {
			AccountID       string `json:"account_id"`
			AccountDisplay  string `json:"account_display_name"`
			AccountLogin    string `json:"account_login"`
			IdentityID      string `json:"identity_id"`
			IdentityDisplay string `json:"identity_display_name"`
			FromEmail       string `json:"from_email"`
			ReplyTo         string `json:"reply_to"`
			SignatureText   string `json:"signature_text"`
			SignatureHTML   string `json:"signature_html"`
			IsDefault       bool   `json:"is_default"`
			IsSession       bool   `json:"is_session"`
		} `json:"items"`
	}
	if err := json.Unmarshal(compose.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode compose identities payload: %v body=%s", err, compose.Body.String())
	}
	if payload.AuthEmail != "admin@example.com" {
		t.Fatalf("expected auth_email admin@example.com, got %+v", payload)
	}
	var sessionItem, identityItem *struct {
		AccountID       string `json:"account_id"`
		AccountDisplay  string `json:"account_display_name"`
		AccountLogin    string `json:"account_login"`
		IdentityID      string `json:"identity_id"`
		IdentityDisplay string `json:"identity_display_name"`
		FromEmail       string `json:"from_email"`
		ReplyTo         string `json:"reply_to"`
		SignatureText   string `json:"signature_text"`
		SignatureHTML   string `json:"signature_html"`
		IsDefault       bool   `json:"is_default"`
		IsSession       bool   `json:"is_session"`
	}
	for i := range payload.Items {
		item := &payload.Items[i]
		if item.IsSession {
			sessionItem = item
		}
		if item.IdentityID == identity.ID {
			identityItem = item
		}
	}
	if sessionItem == nil {
		t.Fatalf("expected session compose identity in payload: %+v", payload.Items)
	}
	if sessionItem.AccountID != "" || sessionItem.IdentityDisplay != "Admin Session" || sessionItem.FromEmail != "admin@example.com" || sessionItem.ReplyTo != "session-reply@example.com" {
		t.Fatalf("unexpected session compose identity: %+v", *sessionItem)
	}
	if sessionItem.SignatureHTML != "<p>Session Signature</p>" || sessionItem.SignatureText != "-- \nSession Signature" {
		t.Fatalf("expected session signature metadata, got %+v", *sessionItem)
	}
	if identityItem == nil {
		t.Fatalf("expected account identity in compose identities payload: %+v", payload.Items)
	}
	if identityItem.AccountID != account.ID || identityItem.AccountDisplay != "Primary Mail" || identityItem.AccountLogin != "mailbox@example.com" {
		t.Fatalf("unexpected account metadata in compose identity: %+v", *identityItem)
	}
	if identityItem.IdentityDisplay != "Primary Alias" || identityItem.FromEmail != "alias@example.com" || identityItem.ReplyTo != "alias-reply@example.com" || identityItem.SignatureHTML != "<p>Account Signature</p>" {
		t.Fatalf("unexpected compose identity metadata: %+v", *identityItem)
	}
	if !identityItem.IsDefault {
		t.Fatalf("expected created identity to be surfaced as default")
	}
}

func TestV2CreateIdentityRejectsInvalidFromEmail(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	createAccount := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts", map[string]any{
		"display_name": "Primary Mail",
		"login":        "mailbox@example.com",
		"password":     "mailbox-secret",
		"imap_host":    "imap.example.com",
		"imap_port":    993,
		"smtp_host":    "smtp.example.com",
		"smtp_port":    587,
	}, sess, csrf)
	if createAccount.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", createAccount.Code, createAccount.Body.String())
	}
	var account models.MailAccount
	if err := json.Unmarshal(createAccount.Body.Bytes(), &account); err != nil {
		t.Fatalf("decode account response: %v", err)
	}

	createIdentity := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts/"+account.ID+"/identities", map[string]any{
		"display_name": "Primary Alias",
		"from_email":   "webmaster",
	}, sess, csrf)
	if createIdentity.Code != http.StatusBadRequest {
		t.Fatalf("expected identity create 400, got %d body=%s", createIdentity.Code, createIdentity.Body.String())
	}
	var apiErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(createIdentity.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode api error: %v body=%s", err, createIdentity.Body.String())
	}
	if apiErr.Code != "create_identity_failed" || apiErr.Message != "from_email must be a valid email address" {
		t.Fatalf("unexpected api error: %+v", apiErr)
	}
}

func TestV2SendDraftSchedulesFutureDraft(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	createAccount := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts", map[string]any{
		"display_name": "Primary",
		"login":        "admin@example.com",
		"password":     "mailbox-secret",
		"imap_host":    "imap.example.com",
		"imap_port":    993,
		"smtp_host":    "smtp.example.com",
		"smtp_port":    587,
	}, sess, csrf)
	if createAccount.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", createAccount.Code, createAccount.Body.String())
	}
	var account models.MailAccount
	if err := json.Unmarshal(createAccount.Body.Bytes(), &account); err != nil {
		t.Fatalf("decode account response: %v", err)
	}

	scheduledFor := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	createDraft := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":    account.ID,
		"to":            "someone@example.com",
		"subject":       "Scheduled draft",
		"body_text":     "Hello future",
		"send_mode":     "scheduled",
		"scheduled_for": scheduledFor,
	}, sess, csrf)
	if createDraft.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", createDraft.Code, createDraft.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(createDraft.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/send", nil, sess, csrf)
	if send.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for scheduled send, got %d body=%s", send.Code, send.Body.String())
	}
}

func TestV2ScheduledDraftQueuesResolvedIdentityAccount(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	primary := createV2TestAccount(t, router, sess, csrf, "primary@example.com")
	secondary := createV2TestAccount(t, router, sess, csrf, "alias@example.com")
	identity := createV2TestIdentity(t, router, sess, csrf, secondary.ID, "alias@example.com")

	scheduledFor := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
	createDraft := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":    primary.ID,
		"identity_id":   identity.ID,
		"from_mode":     "identity",
		"to":            "someone@example.com",
		"subject":       "Scheduled alias",
		"body_text":     "Hello future",
		"send_mode":     "scheduled",
		"scheduled_for": scheduledFor,
	}, sess, csrf)
	if createDraft.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", createDraft.Code, createDraft.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(createDraft.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/send", nil, sess, csrf)
	if send.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for scheduled send, got %d body=%s", send.Code, send.Body.String())
	}

	items, err := st.ListDueScheduledSends(context.Background(), time.Now().UTC().Add(15*time.Minute), 10)
	if err != nil {
		t.Fatalf("list scheduled sends: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one queued scheduled send, got %+v", items)
	}
	if items[0].AccountID != secondary.ID {
		t.Fatalf("expected queue to use resolved identity account %q, got %+v", secondary.ID, items[0])
	}

	storedDraft, err := st.GetDraftByID(context.Background(), draft.UserID, draft.ID)
	if err != nil {
		t.Fatalf("reload scheduled draft: %v", err)
	}
	if storedDraft.AccountID != secondary.ID {
		t.Fatalf("expected scheduled draft account to be normalized to sender account %q, got %+v", secondary.ID, storedDraft)
	}
}

func TestV2DraftCRUDPersistsComposeContext(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":         "",
		"compose_mode":       "reply",
		"context_message_id": "msg-123",
		"from_mode":          "manual",
		"from_manual":        "admin@example.com",
		"client_state_json":  `{"cc_visible":true,"to_pending":"alice@example.com"}`,
		"to":                 "alice@example.com",
		"subject":            "Draft subject",
		"body_text":          "Draft body",
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}
	if draft.AccountID != "" || draft.ComposeMode != "reply" || draft.ContextMessageID != "msg-123" {
		t.Fatalf("unexpected created draft: %+v", draft)
	}

	get := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/drafts/"+draft.ID, nil, sess, csrf)
	if get.Code != http.StatusOK {
		t.Fatalf("expected draft get 200, got %d body=%s", get.Code, get.Body.String())
	}
	var fetched models.Draft
	if err := json.Unmarshal(get.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode fetched draft: %v", err)
	}
	if fetched.ClientStateJSON == "" || fetched.FromMode != "manual" || fetched.FromManual != "admin@example.com" {
		t.Fatalf("expected compose context to round-trip, got %+v", fetched)
	}

	update := doV2AuthedJSON(t, router, http.MethodPatch, "/api/v2/drafts/"+draft.ID, map[string]any{
		"compose_mode":       "forward",
		"context_message_id": "msg-456",
		"subject":            "Updated subject",
	}, sess, csrf)
	if update.Code != http.StatusOK {
		t.Fatalf("expected draft update 200, got %d body=%s", update.Code, update.Body.String())
	}

	del := doV2AuthedJSON(t, router, http.MethodDelete, "/api/v2/drafts/"+draft.ID, nil, sess, csrf)
	if del.Code != http.StatusOK {
		t.Fatalf("expected draft delete 200, got %d body=%s", del.Code, del.Body.String())
	}
	missing := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/drafts/"+draft.ID, nil, sess, csrf)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected deleted draft to return 404, got %d body=%s", missing.Code, missing.Body.String())
	}
}

func TestV2DraftCRUDPersistsContextAccountID(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)
	account := createV2TestAccount(t, router, sess, csrf, "context@example.com")

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"compose_mode":       "reply",
		"context_message_id": "msg-123",
		"context_account_id": account.ID,
		"from_mode":          "manual",
		"from_manual":        "admin@example.com",
		"to":                 "alice@example.com",
		"subject":            "Draft subject",
		"body_text":          "Draft body",
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}
	if draft.ContextAccountID != account.ID {
		t.Fatalf("expected context_account_id to persist, got %+v", draft)
	}

	update := doV2AuthedJSON(t, router, http.MethodPatch, "/api/v2/drafts/"+draft.ID, map[string]any{
		"context_account_id": "",
	}, sess, csrf)
	if update.Code != http.StatusOK {
		t.Fatalf("expected draft update 200, got %d body=%s", update.Code, update.Body.String())
	}
	var updated models.Draft
	if err := json.Unmarshal(update.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated draft: %v", err)
	}
	if updated.ContextAccountID != "" {
		t.Fatalf("expected context_account_id to clear, got %+v", updated)
	}
}

func TestV2CreateDraftAcceptsBlankScheduledForFromComposeClient(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":         "",
		"identity_id":        "",
		"compose_mode":       "send",
		"context_message_id": "",
		"context_account_id": "",
		"from_mode":          "default",
		"from_manual":        "",
		"client_state_json":  `{"cc_visible":false}`,
		"to":                 "alice@example.com",
		"cc":                 "",
		"bcc":                "",
		"subject":            "Draft subject",
		"body_text":          "Draft body",
		"body_html":          "<p>Draft body</p>",
		"send_mode":          "",
		"scheduled_for":      "",
		"status":             "active",
		"last_send_error":    "",
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v body=%s", err, create.Body.String())
	}
	if !draft.ScheduledFor.IsZero() {
		t.Fatalf("expected blank scheduled_for to decode as zero time, got %+v", draft)
	}
}

func TestV2ListDraftVersionsRequiresDraftOwnership(t *testing.T) {
	router, st := newV2RouterWithConfigAndStore(t, nil)
	sess, csrf := loginV2(t, router)
	otherHash, err := auth.HashPassword("OtherPass123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	otherUser, err := st.CreateUser(context.Background(), "other@example.com", otherHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	draft, err := st.CreateDraft(context.Background(), models.Draft{
		UserID:   otherUser.ID,
		ToValue:  "someone@example.com",
		Subject:  "Private draft",
		BodyText: "Private draft body",
		Status:   "active",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	rec := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/drafts/"+draft.ID+"/versions", nil, sess, csrf)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign draft versions, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestV2ListAccountMailboxesAndUpsertSpecialMailbox(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 2, Messages: 7},
		},
	}
	withMailClientFactory(t, func(cfg config.Config) mail.Client { return client })
	router, st := newV2RouterWithMailClientAndStore(t, mail.NoopClient{}, nil)
	sess, csrf := loginV2(t, router)
	account := createV2TestAccount(t, router, sess, csrf, "account@example.com")

	if _, err := st.UpsertMailboxMapping(context.Background(), models.MailboxMapping{
		ID:          "map-inbox",
		AccountID:   account.ID,
		Role:        "archive",
		MailboxName: "Archive",
		Source:      "manual",
		Priority:    100,
	}); err != nil {
		t.Fatalf("seed mailbox mapping: %v", err)
	}

	list := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/accounts/"+account.ID+"/mailboxes", nil, sess, csrf)
	if list.Code != http.StatusOK {
		t.Fatalf("expected mailbox list 200, got %d body=%s", list.Code, list.Body.String())
	}
	var mailboxes []mail.Mailbox
	if err := json.Unmarshal(list.Body.Bytes(), &mailboxes); err != nil {
		t.Fatalf("decode mailbox list: %v", err)
	}
	if len(mailboxes) != 1 || mailboxes[0].Name != "INBOX" {
		t.Fatalf("unexpected account mailboxes: %+v", mailboxes)
	}

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts/"+account.ID+"/mailboxes/special/sent", map[string]any{
		"mailbox_name":      "Sent",
		"create_if_missing": true,
	}, sess, csrf)
	if create.Code != http.StatusOK {
		t.Fatalf("expected special mailbox upsert 200, got %d body=%s", create.Code, create.Body.String())
	}
	var payload struct {
		MailboxName string         `json:"mailbox_name"`
		Created     bool           `json:"created"`
		Mailboxes   []mail.Mailbox `json:"mailboxes"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode special mailbox payload: %v", err)
	}
	if payload.MailboxName != "Sent" || !payload.Created {
		t.Fatalf("expected created Sent mailbox, got %+v", payload)
	}
	if len(client.createdMailboxes) != 1 || client.createdMailboxes[0] != "Sent" {
		t.Fatalf("expected account-scoped mailbox creation, got %+v", client.createdMailboxes)
	}
	mappings, err := st.ListMailboxMappings(context.Background(), account.ID)
	if err != nil {
		t.Fatalf("list mailbox mappings: %v", err)
	}
	if resolveMappedMailboxByRole(mappings, "sent") != "Sent" {
		t.Fatalf("expected sent mailbox mapping to persist, got %+v", mappings)
	}
}

func TestV2SendDraftReplyUsesIndexedContextHeadersWithoutSessionGetMessage(t *testing.T) {
	client := &sendTestDespatch{}
	withMailClientFactory(t, func(cfg config.Config) mail.Client { return client })
	router, st := newV2RouterWithMailClientAndStore(t, client, nil)
	sess, csrf := loginV2(t, router)
	account := createV2TestAccount(t, router, sess, csrf, "indexed@example.com")
	identity := createV2TestIdentity(t, router, sess, csrf, account.ID, "alias@example.com")

	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:               "indexed-reply",
		AccountID:        account.ID,
		Mailbox:          "INBOX",
		UID:              42,
		ThreadID:         mail.ScopeIndexedThreadID(account.ID, "thread-indexed"),
		FromValue:        "Sender <sender@example.com>",
		ToValue:          "alias@example.com",
		Subject:          "Indexed reply",
		Snippet:          "Indexed body",
		BodyText:         "Indexed body",
		MessageIDHeader:  "<indexed-original@example.com>",
		ReferencesHeader: `["<root@example.com>"]`,
		InReplyToHeader:  "<parent@example.com>",
		DateHeader:       time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
		InternalDate:     time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
	})

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":         account.ID,
		"identity_id":        identity.ID,
		"compose_mode":       "reply",
		"context_message_id": "indexed-reply",
		"context_account_id": account.ID,
		"from_mode":          "identity",
		"to":                 "sender@example.com",
		"subject":            "Re: Indexed reply",
		"body_text":          "Thanks",
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/send", nil, sess, csrf)
	if send.Code != http.StatusOK {
		t.Fatalf("expected draft send 200, got %d body=%s", send.Code, send.Body.String())
	}
	if client.getMessageCallCount() != 0 {
		t.Fatalf("expected indexed reply send to avoid session GetMessage, got %d calls", client.getMessageCallCount())
	}
	user, req := client.snapshot()
	if user != account.Login {
		t.Fatalf("expected account-backed send user %q, got %q", account.Login, user)
	}
	if req.InReplyToID != "indexed-original@example.com" {
		t.Fatalf("expected indexed in-reply-to header, got %+v", req)
	}
	if len(req.References) != 2 || req.References[0] != "root@example.com" || req.References[1] != "indexed-original@example.com" {
		t.Fatalf("expected indexed references header, got %+v", req.References)
	}
}

func TestV2SendDraftReplyWithoutContextAccountUsesSessionFallback(t *testing.T) {
	client := &sendTestDespatch{
		messageByID: map[string]mail.Message{
			"legacy-msg": {
				ID:         "legacy-msg",
				MessageID:  "<legacy@example.com>",
				References: []string{"<seed@example.com>"},
			},
		},
	}
	router := newV2RouterWithMailClient(t, client, nil)
	sess, csrf := loginV2(t, router)

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"compose_mode":       "reply",
		"context_message_id": "legacy-msg",
		"from_mode":          "manual",
		"from_manual":        "admin@example.com",
		"to":                 "legacy@example.com",
		"subject":            "Re: Legacy reply",
		"body_text":          "Thanks",
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/send", nil, sess, csrf)
	if send.Code != http.StatusOK {
		t.Fatalf("expected draft send 200, got %d body=%s", send.Code, send.Body.String())
	}
	if client.getMessageCallCount() != 1 {
		t.Fatalf("expected session fallback GetMessage call, got %d", client.getMessageCallCount())
	}
	_, req := client.snapshot()
	if req.InReplyToID != "legacy@example.com" {
		t.Fatalf("expected session fallback in-reply-to header, got %+v", req)
	}
	if len(req.References) != 2 || req.References[0] != "seed@example.com" || req.References[1] != "legacy@example.com" {
		t.Fatalf("expected session fallback references, got %+v", req.References)
	}
}

func TestV2DraftUpdateClearsScheduledFor(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	scheduledFor := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"to":            "someone@example.com",
		"subject":       "Schedule me",
		"body_text":     "Hello",
		"send_mode":     "scheduled",
		"scheduled_for": scheduledFor,
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}
	if draft.ScheduledFor.IsZero() {
		t.Fatalf("expected scheduled draft to keep scheduled_for, got %+v", draft)
	}

	update := doV2AuthedJSON(t, router, http.MethodPatch, "/api/v2/drafts/"+draft.ID, map[string]any{
		"send_mode":     "",
		"scheduled_for": "",
	}, sess, csrf)
	if update.Code != http.StatusOK {
		t.Fatalf("expected draft update 200, got %d body=%s", update.Code, update.Body.String())
	}
	var updated models.Draft
	if err := json.Unmarshal(update.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated draft: %v", err)
	}
	if !updated.ScheduledFor.IsZero() {
		t.Fatalf("expected scheduled_for to clear, got %+v", updated)
	}
}

func TestV2SendDraftWithoutAccountMarksDraftSent(t *testing.T) {
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":   "",
		"to":           "someone@example.com",
		"subject":      "Send me",
		"body_text":    "Hello",
		"compose_mode": "send",
		"from_mode":    "manual",
		"from_manual":  "admin@example.com",
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/send", nil, sess, csrf)
	if send.Code != http.StatusOK {
		t.Fatalf("expected draft send 200, got %d body=%s", send.Code, send.Body.String())
	}

	list := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/drafts?page=1&page_size=20", nil, sess, csrf)
	if list.Code != http.StatusOK {
		t.Fatalf("expected draft list 200, got %d body=%s", list.Code, list.Body.String())
	}
	var payload struct {
		Items []models.Draft `json:"items"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode draft list: %v body=%s", err, list.Body.String())
	}
	for _, item := range payload.Items {
		if item.ID == draft.ID {
			t.Fatalf("expected sent draft %q to be excluded from active list", draft.ID)
		}
	}
}

func TestV2SendDraftUsesSessionProfileSenderHeaders(t *testing.T) {
	despatch := &sendTestDespatch{}
	router := newV2RouterWithMailClient(t, despatch, nil)
	sess, csrf := loginV2(t, router)

	updateProfile := doV2AuthedJSON(t, router, http.MethodPatch, "/api/v2/mail/session-profile", map[string]any{
		"display_name":   "Admin Session",
		"reply_to":       "reply@example.com",
		"signature_html": "<p>Regards</p>",
	}, sess, csrf)
	if updateProfile.Code != http.StatusOK {
		t.Fatalf("expected session profile patch 200, got %d body=%s", updateProfile.Code, updateProfile.Body.String())
	}

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":   "",
		"to":           "someone@example.com",
		"subject":      "Send me",
		"body_text":    "Hello",
		"compose_mode": "send",
		"from_mode":    "default",
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/send", nil, sess, csrf)
	if send.Code != http.StatusOK {
		t.Fatalf("expected draft send 200, got %d body=%s", send.Code, send.Body.String())
	}

	_, req := despatch.snapshot()
	if req.HeaderFromName != "Admin Session" || req.HeaderFromEmail != "admin@example.com" {
		t.Fatalf("expected session profile sender headers, got %#v", req)
	}
	if req.EnvelopeFrom != "admin@example.com" || req.ReplyTo != "reply@example.com" || req.From != "admin@example.com" {
		t.Fatalf("expected session profile reply/envelope headers, got %#v", req)
	}
}

func TestV2SendDraftIdentityModeUsesAccountIdentityHeaders(t *testing.T) {
	accountClient := &sendTestDespatch{}
	withMailClientFactory(t, func(cfg config.Config) mail.Client {
		return accountClient
	})
	router := newV2Router(t)
	sess, csrf := loginV2(t, router)

	createAccount := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts", map[string]any{
		"display_name": "Primary Mail",
		"login":        "mailbox@example.com",
		"password":     "mailbox-secret",
		"imap_host":    "imap.example.com",
		"imap_port":    993,
		"smtp_host":    "smtp.example.com",
		"smtp_port":    587,
	}, sess, csrf)
	if createAccount.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", createAccount.Code, createAccount.Body.String())
	}
	var account models.MailAccount
	if err := json.Unmarshal(createAccount.Body.Bytes(), &account); err != nil {
		t.Fatalf("decode account response: %v", err)
	}

	createIdentity := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts/"+account.ID+"/identities", map[string]any{
		"display_name":   "Primary Alias",
		"from_email":     "alias@example.com",
		"reply_to":       "alias-reply@example.com",
		"signature_html": "<p>Account Signature</p>",
		"is_default":     true,
	}, sess, csrf)
	if createIdentity.Code != http.StatusCreated {
		t.Fatalf("expected identity create 201, got %d body=%s", createIdentity.Code, createIdentity.Body.String())
	}
	var identity models.MailIdentity
	if err := json.Unmarshal(createIdentity.Body.Bytes(), &identity); err != nil {
		t.Fatalf("decode identity response: %v", err)
	}

	createDraft := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":   account.ID,
		"identity_id":  identity.ID,
		"from_mode":    "identity",
		"to":           "someone@example.com",
		"subject":      "Alias send",
		"body_text":    "Hello",
		"compose_mode": "send",
	}, sess, csrf)
	if createDraft.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", createDraft.Code, createDraft.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(createDraft.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/send", nil, sess, csrf)
	if send.Code != http.StatusOK {
		t.Fatalf("expected draft send 200, got %d body=%s", send.Code, send.Body.String())
	}

	user, req := accountClient.snapshot()
	if user != "mailbox@example.com" {
		t.Fatalf("expected account login mailbox@example.com, got %q", user)
	}
	if req.HeaderFromName != "Primary Alias" || req.HeaderFromEmail != "alias@example.com" {
		t.Fatalf("expected identity sender headers, got %#v", req)
	}
	if req.EnvelopeFrom != "alias@example.com" || req.ReplyTo != "alias-reply@example.com" || req.From != "alias@example.com" {
		t.Fatalf("expected identity envelope/reply headers, got %#v", req)
	}
}

func TestV2DraftAttachmentUploadRoundTripsAndSendUsesStoredMedia(t *testing.T) {
	despatch := &sendTestDespatch{}
	router := newV2RouterWithMailClient(t, despatch, nil)
	sess, csrf := loginV2(t, router)

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":   "",
		"to":           "someone@example.com",
		"subject":      "Send me with files",
		"body_text":    "Hello",
		"body_html":    "<p>Hello</p>",
		"compose_mode": "send",
		"from_mode":    "manual",
		"from_manual":  "admin@example.com",
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}

	upload := doV2AuthedMultipart(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/attachments", func(w *multipart.Writer) {
		file, err := w.CreateFormFile("attachments", "notes.txt")
		if err != nil {
			t.Fatalf("create attachment part: %v", err)
		}
		if _, err := file.Write([]byte("draft attachment")); err != nil {
			t.Fatalf("write attachment part: %v", err)
		}
		inline, err := w.CreateFormFile("inline_images", "inline.png")
		if err != nil {
			t.Fatalf("create inline part: %v", err)
		}
		if _, err := inline.Write([]byte("PNGDATA")); err != nil {
			t.Fatalf("write inline part: %v", err)
		}
		if err := w.WriteField("inline_image_cids", "cid-inline-1"); err != nil {
			t.Fatalf("write cid field: %v", err)
		}
	}, sess, csrf)
	if upload.Code != http.StatusCreated {
		t.Fatalf("expected attachment upload 201, got %d body=%s", upload.Code, upload.Body.String())
	}
	var uploadPayload struct {
		Draft    models.Draft             `json:"draft"`
		Items    []models.DraftAttachment `json:"items"`
		Uploaded []models.DraftAttachment `json:"uploaded"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &uploadPayload); err != nil {
		t.Fatalf("decode upload response: %v body=%s", err, upload.Body.String())
	}
	if len(uploadPayload.Items) != 2 || len(uploadPayload.Uploaded) != 2 {
		t.Fatalf("expected 2 stored draft attachments, got items=%d uploaded=%d", len(uploadPayload.Items), len(uploadPayload.Uploaded))
	}
	contentID := ""
	attachmentID := ""
	for _, item := range uploadPayload.Items {
		if item.InlinePart {
			contentID = item.ContentID
			continue
		}
		attachmentID = item.ID
	}
	if contentID != "cid-inline-1" {
		t.Fatalf("expected inline content id cid-inline-1, got %q", contentID)
	}
	if attachmentID == "" {
		t.Fatalf("expected regular attachment id in upload response")
	}
	if !strings.Contains(uploadPayload.Draft.AttachmentsJSON, attachmentID) {
		t.Fatalf("expected attachments_json to reference uploaded attachment, got %q", uploadPayload.Draft.AttachmentsJSON)
	}

	getAttachment := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/drafts/"+draft.ID+"/attachments/"+attachmentID, nil, sess, csrf)
	if getAttachment.Code != http.StatusOK {
		t.Fatalf("expected draft attachment get 200, got %d body=%s", getAttachment.Code, getAttachment.Body.String())
	}
	if body := getAttachment.Body.String(); body != "draft attachment" {
		t.Fatalf("expected stored attachment body, got %q", body)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/send", nil, sess, csrf)
	if send.Code != http.StatusOK {
		t.Fatalf("expected draft send 200, got %d body=%s", send.Code, send.Body.String())
	}
	_, req := despatch.snapshot()
	if len(req.Attachments) != 2 {
		t.Fatalf("expected 2 stored attachments on send request, got %d", len(req.Attachments))
	}
	var sawInline, sawFile bool
	for _, item := range req.Attachments {
		if item.Inline {
			sawInline = item.ContentID == "cid-inline-1" && string(item.Data) == "PNGDATA"
			continue
		}
		sawFile = item.Filename == "notes.txt" && string(item.Data) == "draft attachment"
	}
	if !sawInline || !sawFile {
		t.Fatalf("expected stored inline and file attachments to be sent, got %#v", req.Attachments)
	}
}

func TestV2SendDraftFailurePersistsFailedState(t *testing.T) {
	despatch := &sendTestDespatch{sendErr: errors.New("temporary smtp outage")}
	router := newV2RouterWithMailClient(t, despatch, nil)
	sess, csrf := loginV2(t, router)

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":   "",
		"to":           "someone@example.com",
		"subject":      "Retry me",
		"body_text":    "Hello",
		"compose_mode": "send",
		"from_mode":    "manual",
		"from_manual":  "admin@example.com",
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/send", nil, sess, csrf)
	if send.Code != http.StatusBadGateway {
		t.Fatalf("expected draft send 502, got %d body=%s", send.Code, send.Body.String())
	}

	get := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/drafts/"+draft.ID, nil, sess, csrf)
	if get.Code != http.StatusOK {
		t.Fatalf("expected draft get 200, got %d body=%s", get.Code, get.Body.String())
	}
	var failedDraft models.Draft
	if err := json.Unmarshal(get.Body.Bytes(), &failedDraft); err != nil {
		t.Fatalf("decode failed draft: %v body=%s", err, get.Body.String())
	}
	if failedDraft.Status != "failed" {
		t.Fatalf("expected failed draft status, got %+v", failedDraft)
	}
	if !strings.Contains(strings.ToLower(failedDraft.LastSendError), "temporary smtp outage") {
		t.Fatalf("expected last_send_error to persist send failure, got %q", failedDraft.LastSendError)
	}
}

func TestV2SendDraftSMTPPolicyFailureDoesNotPersistFailedState(t *testing.T) {
	despatch := &sendTestDespatch{sendErr: mail.ErrSMTPSenderRejected}
	router := newV2RouterWithMailClient(t, despatch, nil)
	sess, csrf := loginV2(t, router)

	create := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts", map[string]any{
		"account_id":   "",
		"to":           "someone@example.com",
		"subject":      "Sender check",
		"body_text":    "Hello",
		"compose_mode": "send",
		"from_mode":    "manual",
		"from_manual":  "admin@example.com",
	}, sess, csrf)
	if create.Code != http.StatusCreated {
		t.Fatalf("expected draft create 201, got %d body=%s", create.Code, create.Body.String())
	}
	var draft models.Draft
	if err := json.Unmarshal(create.Body.Bytes(), &draft); err != nil {
		t.Fatalf("decode draft response: %v", err)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/drafts/"+draft.ID+"/send", nil, sess, csrf)
	if send.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected draft send 422, got %d body=%s", send.Code, send.Body.String())
	}

	get := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/drafts/"+draft.ID, nil, sess, csrf)
	if get.Code != http.StatusOK {
		t.Fatalf("expected draft get 200, got %d body=%s", get.Code, get.Body.String())
	}
	var current models.Draft
	if err := json.Unmarshal(get.Body.Bytes(), &current); err != nil {
		t.Fatalf("decode draft: %v body=%s", err, get.Body.String())
	}
	if current.Status == "failed" {
		t.Fatalf("expected smtp sender rejection to keep draft editable, got %+v", current)
	}
	if current.LastSendError != "" {
		t.Fatalf("expected smtp sender rejection not to persist last_send_error, got %q", current.LastSendError)
	}
}

func TestV2SendMessageAppliesPGPCryptoPipeline(t *testing.T) {
	socketPath, recorder := startFakeMailSecServerWithRecorder(t)
	router := newV2RouterWithConfig(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
	})
	sess, csrf := loginV2(t, router)

	createSign := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/crypto/keyrings", map[string]any{
		"kind":        "pgp_private",
		"public_key":  "PGP-PUB-SIGN",
		"private_key": "PGP-PRIV-SIGN",
		"user_ids":    []string{"admin@example.com"},
	}, sess, csrf)
	if createSign.Code != http.StatusCreated {
		t.Fatalf("expected signing keyring create 201, got %d body=%s", createSign.Code, createSign.Body.String())
	}
	var signKey models.CryptoKeyring
	if err := json.Unmarshal(createSign.Body.Bytes(), &signKey); err != nil {
		t.Fatalf("decode signing keyring: %v", err)
	}

	createRecipient := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/crypto/keyrings", map[string]any{
		"kind":       "pgp_public",
		"public_key": "PGP-PUB-RECIPIENT",
		"user_ids":   []string{"recipient@example.com"},
	}, sess, csrf)
	if createRecipient.Code != http.StatusCreated {
		t.Fatalf("expected recipient keyring create 201, got %d body=%s", createRecipient.Code, createRecipient.Body.String())
	}
	var recipientKey models.CryptoKeyring
	if err := json.Unmarshal(createRecipient.Body.Bytes(), &recipientKey); err != nil {
		t.Fatalf("decode recipient keyring: %v", err)
	}

	send := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/messages/send", map[string]any{
		"to":      []string{"recipient@example.com"},
		"subject": "Secure hello",
		"body":    "hello world",
		"crypto_options": map[string]any{
			"provider":              "pgp",
			"sign":                  true,
			"encrypt":               true,
			"sign_keyring_id":       signKey.ID,
			"recipient_keyring_ids": []string{recipientKey.ID},
		},
		"crypto_passphrase": "hunter2",
	}, sess, csrf)
	if send.Code != http.StatusOK {
		t.Fatalf("expected send 200, got %d body=%s", send.Code, send.Body.String())
	}

	signOps := recorder.byOp("crypto.pgp.sign")
	if len(signOps) != 1 {
		t.Fatalf("expected 1 pgp sign op, got %d", len(signOps))
	}
	if got, _ := signOps[0].Payload["plaintext"].(string); got != "hello world" {
		t.Fatalf("expected sign plaintext hello world, got %q", got)
	}
	encryptOps := recorder.byOp("crypto.pgp.encrypt")
	if len(encryptOps) != 1 {
		t.Fatalf("expected 1 pgp encrypt op, got %d", len(encryptOps))
	}
	encryptPlaintext, _ := encryptOps[0].Payload["plaintext"].(string)
	if !strings.HasPrefix(encryptPlaintext, "PGP-SIGNED:") {
		t.Fatalf("expected encrypted plaintext to be signed output, got %q", encryptPlaintext)
	}
}

func TestV2DecryptAndVerifyIndexedMessageWithSMIME(t *testing.T) {
	socketPath, recorder := startFakeMailSecServerWithRecorder(t)
	router, st, sqdb := newV2RouterWithConfigAndStoreDB(t, func(cfg *config.Config) {
		cfg.MailSecEnabled = true
		cfg.MailSecSocket = socketPath
		cfg.MailSecTimeoutMS = 2000
	})
	sess, csrf := loginV2(t, router)

	createAccount := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/accounts", map[string]any{
		"display_name": "Primary",
		"login":        "admin@example.com",
		"password":     "mailbox-secret",
		"imap_host":    "imap.example.com",
		"imap_port":    993,
		"smtp_host":    "smtp.example.com",
		"smtp_port":    587,
	}, sess, csrf)
	if createAccount.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", createAccount.Code, createAccount.Body.String())
	}
	var account models.MailAccount
	if err := json.Unmarshal(createAccount.Body.Bytes(), &account); err != nil {
		t.Fatalf("decode account: %v", err)
	}

	createPrivate := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/crypto/keyrings", map[string]any{
		"account_id":      account.ID,
		"kind":            "smime_private",
		"public_key":      "SMIME-CERT-PRIVATE",
		"private_key":     "SMIME-PRIVATE-KEY",
		"trust_level":     "high",
		"fingerprint":     "smime-private-fpr",
		"passphrase_hint": "test",
	}, sess, csrf)
	if createPrivate.Code != http.StatusCreated {
		t.Fatalf("expected private keyring create 201, got %d body=%s", createPrivate.Code, createPrivate.Body.String())
	}
	var privateKey models.CryptoKeyring
	if err := json.Unmarshal(createPrivate.Body.Bytes(), &privateKey); err != nil {
		t.Fatalf("decode private keyring: %v", err)
	}

	createPublic := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/security/crypto/keyrings", map[string]any{
		"account_id":  account.ID,
		"kind":        "smime_public",
		"public_key":  "SMIME-CERT-PUBLIC",
		"trust_level": "high",
	}, sess, csrf)
	if createPublic.Code != http.StatusCreated {
		t.Fatalf("expected public keyring create 201, got %d body=%s", createPublic.Code, createPublic.Body.String())
	}
	var publicKey models.CryptoKeyring
	if err := json.Unmarshal(createPublic.Body.Bytes(), &publicKey); err != nil {
		t.Fatalf("decode public keyring: %v", err)
	}

	now := time.Now().UTC()
	scopedMessageID := mail.ScopeIndexedMessageID(account.ID, "msg-crypto-1")
	scopedThreadID := mail.ScopeIndexedThreadID(account.ID, "thread-1")
	if _, err := sqdb.ExecContext(context.Background(),
		`INSERT INTO thread_index(
		  id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		scopedThreadID,
		account.ID,
		"Inbox",
		"encrypted",
		`["sender@example.com","admin@example.com"]`,
		1,
		1,
		0,
		0,
		0,
		scopedMessageID,
		now,
		now,
	); err != nil {
		t.Fatalf("insert thread index row: %v", err)
	}

	msg, err := st.UpsertIndexedMessage(context.Background(), models.IndexedMessage{
		ID:         scopedMessageID,
		AccountID:  account.ID,
		Mailbox:    "Inbox",
		UID:        1,
		ThreadID:   scopedThreadID,
		FromValue:  "sender@example.com",
		ToValue:    "admin@example.com",
		Subject:    "Encrypted",
		Snippet:    "Encrypted payload",
		BodyText:   "",
		RawSource:  "SMIME-CIPHERTEXT",
		Seen:       false,
		Importance: 0,
	})
	if err != nil {
		t.Fatalf("upsert indexed message: %v", err)
	}

	decrypt := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/messages/"+msg.ID+"/crypto/decrypt?account_id="+account.ID, map[string]any{
		"provider":   "smime",
		"keyring_id": privateKey.ID,
		"passphrase": "hunter2",
	}, sess, csrf)
	if decrypt.Code != http.StatusOK {
		t.Fatalf("expected decrypt 200, got %d body=%s", decrypt.Code, decrypt.Body.String())
	}
	var decryptBody map[string]any
	if err := json.Unmarshal(decrypt.Body.Bytes(), &decryptBody); err != nil {
		t.Fatalf("decode decrypt response: %v", err)
	}
	resultMap, _ := decryptBody["result"].(map[string]any)
	if got, _ := resultMap["plaintext_utf8"].(string); got != "SMIME-DEC:SMIME-CIPHERTEXT" {
		t.Fatalf("unexpected decrypt plaintext: %q", got)
	}

	verify := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/messages/"+msg.ID+"/crypto/verify?account_id="+account.ID, map[string]any{
		"provider":          "smime",
		"public_keyring_id": publicKey.ID,
	}, sess, csrf)
	if verify.Code != http.StatusOK {
		t.Fatalf("expected verify 200, got %d body=%s", verify.Code, verify.Body.String())
	}

	decryptOps := recorder.byOp("crypto.smime.decrypt")
	if len(decryptOps) != 1 {
		t.Fatalf("expected 1 smime decrypt op, got %d", len(decryptOps))
	}
	if got, _ := decryptOps[0].Payload["ciphertext_smime"].(string); got != "SMIME-CIPHERTEXT" {
		t.Fatalf("unexpected decrypt ciphertext payload: %q", got)
	}
	verifyOps := recorder.byOp("crypto.smime.verify")
	if len(verifyOps) != 1 {
		t.Fatalf("expected 1 smime verify op, got %d", len(verifyOps))
	}
}
