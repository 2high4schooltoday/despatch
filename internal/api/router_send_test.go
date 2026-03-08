package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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

type sendTestDespatch struct {
	mu           sync.Mutex
	sendErr      error
	sendResult   mail.SendResult
	capturedReq  mail.SendRequest
	capturedUser string
	messageByID  map[string]mail.Message
	patches      map[string]mail.FlagPatch
}

func (m *sendTestDespatch) ListMailboxes(ctx context.Context, user, pass string) ([]mail.Mailbox, error) {
	return []mail.Mailbox{{Name: "INBOX", Messages: 1}}, nil
}

func (m *sendTestDespatch) ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]mail.MessageSummary, error) {
	return nil, nil
}

func (m *sendTestDespatch) GetMessage(ctx context.Context, user, pass, id string) (mail.Message, error) {
	if msg, ok := m.messageByID[id]; ok {
		return msg, nil
	}
	return mail.Message{}, nil
}

func (m *sendTestDespatch) Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]mail.MessageSummary, error) {
	return nil, nil
}

func (m *sendTestDespatch) Send(ctx context.Context, user, pass string, req mail.SendRequest) (mail.SendResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capturedUser = user
	m.capturedReq = req
	if m.sendErr != nil {
		return mail.SendResult{}, m.sendErr
	}
	if !m.sendResult.SavedCopy && m.sendResult.Warning == "" && m.sendResult.SavedCopyMailbox == "" {
		return mail.SendResult{SavedCopy: true, SavedCopyMailbox: "Sent"}, nil
	}
	return m.sendResult, nil
}

func (m *sendTestDespatch) SetFlags(ctx context.Context, user, pass, id string, flags []string) error {
	return nil
}

func (m *sendTestDespatch) UpdateFlags(ctx context.Context, user, pass, id string, patch mail.FlagPatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.patches == nil {
		m.patches = map[string]mail.FlagPatch{}
	}
	m.patches[id] = patch
	return nil
}

func (m *sendTestDespatch) Move(ctx context.Context, user, pass, id, mailbox string) error {
	return nil
}

func (m *sendTestDespatch) GetAttachment(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentContent, error) {
	return mail.AttachmentContent{}, nil
}

func (m *sendTestDespatch) GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentMeta, io.ReadCloser, error) {
	return mail.AttachmentMeta{}, nil, errors.New("not implemented")
}

func (m *sendTestDespatch) snapshot() (string, mail.SendRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.capturedUser, m.capturedReq
}

func (m *sendTestDespatch) patchFor(id string) (mail.FlagPatch, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	patch, ok := m.patches[id]
	return patch, ok
}

const sendTestSessionEncryptKey = "this_is_a_valid_long_session_encrypt_key_123456"

func newSendRouterWithStore(t *testing.T, despatch mail.Client, mailLogin string) (http.Handler, *store.Store, config.Config) {
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
	if mailLogin != "" {
		admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
		if err != nil {
			t.Fatalf("load admin: %v", err)
		}
		if err := st.UpdateUserMailLogin(context.Background(), admin.ID, mailLogin); err != nil {
			t.Fatalf("set mail_login: %v", err)
		}
	}

	cfg := config.Config{
		ListenAddr:          ":8080",
		BaseDomain:          "example.com",
		SessionCookieName:   "despatch_session",
		CSRFCookieName:      "despatch_csrf",
		SessionIdleMinutes:  30,
		SessionAbsoluteHour: 24,
		SessionEncryptKey:   sendTestSessionEncryptKey,
		CookieSecureMode:    "never",
		TrustProxy:          false,
		PasswordMinLength:   12,
		PasswordMaxLength:   128,
		DovecotAuthMode:     "sql",
	}

	svc := service.New(cfg, st, despatch, mail.NoopProvisioner{}, nil)
	return NewRouter(cfg, svc), st, cfg
}

func newSendRouter(t *testing.T, despatch mail.Client, mailLogin string) http.Handler {
	t.Helper()
	router, _, _ := newSendRouterWithStore(t, despatch, mailLogin)
	return router
}

func loginForSend(t *testing.T, router http.Handler) (*http.Cookie, *http.Cookie) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"email":    "admin@example.com",
		"password": "SecretPass123!",
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
	if sessionCookie == nil {
		t.Fatalf("missing session cookie")
	}
	if csrfCookie == nil {
		t.Fatalf("missing csrf cookie")
	}
	return sessionCookie, csrfCookie
}

func postSendJSON(t *testing.T, router http.Handler, sessionCookie, csrfCookie *http.Cookie, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	return postMailJSON(t, router, "/api/v1/messages/send", sessionCookie, csrfCookie, body)
}

func postMailJSON(t *testing.T, router http.Handler, path string, sessionCookie, csrfCookie *http.Cookie, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestSendSMTPPolicyErrorMappedTo422(t *testing.T) {
	despatch := &sendTestDespatch{
		sendErr: mail.WrapSMTPSenderRejected(errors.New("sender address rejected: not owned by user")),
	}
	router := newSendRouter(t, despatch, "webmaster")
	sessionCookie, csrfCookie := loginForSend(t, router)

	body, _ := json.Marshal(map[string]any{
		"to":      []string{"alice@example.com"},
		"subject": "hello",
		"body":    "world",
	})
	rec := postSendJSON(t, router, sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode api error: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "smtp_sender_rejected" {
		t.Fatalf("expected smtp_sender_rejected, got %q body=%s", apiErr.Code, rec.Body.String())
	}

	user, req := despatch.snapshot()
	if user != "webmaster" {
		t.Fatalf("expected SMTP auth user to use mail_login, got %q", user)
	}
	if req.From != "admin@example.com" {
		t.Fatalf("expected forced From header admin@example.com, got %q", req.From)
	}
}

func TestSendGenericSMTPErrorMappedTo502(t *testing.T) {
	despatch := &sendTestDespatch{
		sendErr: errors.New("upstream smtp timeout"),
	}
	router := newSendRouter(t, despatch, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	body, _ := json.Marshal(map[string]any{
		"to":      []string{"alice@example.com"},
		"subject": "hello",
		"body":    "world",
	})
	rec := postSendJSON(t, router, sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode api error: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "smtp_error" {
		t.Fatalf("expected smtp_error, got %q body=%s", apiErr.Code, rec.Body.String())
	}
}

func TestSendIgnoresClientFromField(t *testing.T) {
	despatch := &sendTestDespatch{}
	router := newSendRouter(t, despatch, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	body, _ := json.Marshal(map[string]any{
		"from":    "attacker@example.net",
		"to":      []string{"alice@example.com"},
		"subject": "hello",
		"body":    "world",
	})
	rec := postSendJSON(t, router, sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	user, req := despatch.snapshot()
	if user != "admin@example.com" {
		t.Fatalf("expected SMTP auth user to default to account email, got %q", user)
	}
	if req.From != "admin@example.com" {
		t.Fatalf("expected forced From header admin@example.com, got %q", req.From)
	}
}

func TestSendJSONSupportsCCBCCAndBodyHTML(t *testing.T) {
	despatch := &sendTestDespatch{}
	router := newSendRouter(t, despatch, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	body, _ := json.Marshal(map[string]any{
		"to":        []string{"alice@example.com"},
		"cc":        []string{"copy@example.com"},
		"bcc":       []string{"hidden@example.com"},
		"subject":   "rich",
		"body":      "plain",
		"body_html": "<p><strong>rich</strong></p>",
	})
	rec := postSendJSON(t, router, sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	_, req := despatch.snapshot()
	if got := strings.Join(req.CC, ","); got != "copy@example.com" {
		t.Fatalf("expected cc captured, got %q", got)
	}
	if got := strings.Join(req.BCC, ","); got != "hidden@example.com" {
		t.Fatalf("expected bcc captured, got %q", got)
	}
	if req.BodyHTML == "" {
		t.Fatalf("expected body_html to be forwarded")
	}
}

func TestReplyUsesOriginalRFCMessageIDAndMarksOriginalAnswered(t *testing.T) {
	originalID := mail.EncodeMessageID("INBOX", 1)
	despatch := &sendTestDespatch{
		sendResult: mail.SendResult{
			SavedCopy:        false,
			SavedCopyMailbox: "Sent Messages",
			Warning:          "Message sent, but the Sent copy could not be saved.",
		},
		messageByID: map[string]mail.Message{
			originalID: {
				ID:        originalID,
				MessageID: "orig-message@example.com",
				References: []string{
					"older@example.com",
					"orig-message@example.com",
				},
			},
		},
	}
	router := newSendRouter(t, despatch, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	body, _ := json.Marshal(map[string]any{
		"to":      []string{"alice@example.com"},
		"subject": "reply",
		"body":    "world",
	})
	rec := postMailJSON(t, router, "/api/v1/messages/"+originalID+"/reply", sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Status           string `json:"status"`
		SavedCopy        bool   `json:"saved_copy"`
		SavedCopyMailbox string `json:"saved_copy_mailbox"`
		Warning          string `json:"warning"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode reply payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Status != "sent" {
		t.Fatalf("expected status sent, got %+v", payload)
	}
	if payload.SavedCopy {
		t.Fatalf("expected saved_copy=false warning payload, got %+v", payload)
	}
	if payload.Warning == "" {
		t.Fatalf("expected warning payload when sent copy save fails")
	}

	_, req := despatch.snapshot()
	if req.InReplyToID != "orig-message@example.com" {
		t.Fatalf("expected RFC Message-ID in reply request, got %q", req.InReplyToID)
	}
	if len(req.References) != 2 || req.References[0] != "older@example.com" || req.References[1] != "orig-message@example.com" {
		t.Fatalf("unexpected references: %#v", req.References)
	}
	patch, ok := despatch.patchFor(originalID)
	if !ok {
		t.Fatalf("expected answered flag patch to be issued")
	}
	if len(patch.Add) != 1 || patch.Add[0] != "\\Answered" {
		t.Fatalf("unexpected answered patch: %#v", patch)
	}
}

func TestSetMessageFlagsSupportsPatchSemantics(t *testing.T) {
	messageID := mail.EncodeMessageID("INBOX", 1)
	despatch := &sendTestDespatch{}
	router := newSendRouter(t, despatch, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	body, _ := json.Marshal(map[string]any{
		"add":    []string{"\\Seen"},
		"remove": []string{"\\Flagged"},
	})
	rec := postMailJSON(t, router, "/api/v1/messages/"+messageID+"/flags", sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	patch, ok := despatch.patchFor(messageID)
	if !ok {
		t.Fatalf("expected patch flags call")
	}
	if len(patch.Add) != 1 || patch.Add[0] != "\\Seen" {
		t.Fatalf("unexpected add patch: %#v", patch)
	}
	if len(patch.Remove) != 1 || patch.Remove[0] != "\\Flagged" {
		t.Fatalf("unexpected remove patch: %#v", patch)
	}
}

func TestSendManualFromRequiresAuthenticatedEmail(t *testing.T) {
	despatch := &sendTestDespatch{}
	router := newSendRouter(t, despatch, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	body, _ := json.Marshal(map[string]any{
		"to":          []string{"alice@example.com"},
		"subject":     "manual",
		"body":        "body",
		"from_mode":   "manual",
		"from_manual": "spoof@example.net",
	})
	rec := postSendJSON(t, router, sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode api error: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "invalid_sender_manual" {
		t.Fatalf("expected invalid_sender_manual, got %q", apiErr.Code)
	}

	okBody, _ := json.Marshal(map[string]any{
		"to":          []string{"alice@example.com"},
		"subject":     "manual",
		"body":        "body",
		"from_mode":   "manual",
		"from_manual": "admin@example.com",
	})
	okRec := postSendJSON(t, router, sessionCookie, csrfCookie, okBody)
	if okRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", okRec.Code, okRec.Body.String())
	}
	_, req := despatch.snapshot()
	if req.From != "admin@example.com" {
		t.Fatalf("expected forced manual sender admin@example.com, got %q", req.From)
	}

	autoBody, _ := json.Marshal(map[string]any{
		"to":        []string{"alice@example.com"},
		"subject":   "manual-auto",
		"body":      "body",
		"from_mode": "manual",
	})
	autoRec := postSendJSON(t, router, sessionCookie, csrfCookie, autoBody)
	if autoRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", autoRec.Code, autoRec.Body.String())
	}
	_, autoReq := despatch.snapshot()
	if autoReq.From != "admin@example.com" {
		t.Fatalf("expected auto-manual sender fallback admin@example.com, got %q", autoReq.From)
	}
}

func TestSendIdentityModeUsesAccountIdentity(t *testing.T) {
	smtpCapture := startFakeSMTPServer(t)
	despatch := &sendTestDespatch{}
	router, st, cfg := newSendRouterWithStore(t, despatch, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	adminUser, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin user: %v", err)
	}
	secretEnc, err := util.EncryptString(util.Derive32ByteKey(cfg.SessionEncryptKey), "mailbox-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	host, portStr, err := net.SplitHostPort(smtpCapture.addr)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	account, err := st.CreateMailAccount(context.Background(), models.MailAccount{
		ID:           "acct-admin",
		UserID:       adminUser.ID,
		DisplayName:  "Primary",
		Login:        "mailbox@example.com",
		SecretEnc:    secretEnc,
		IMAPHost:     "imap.example.com",
		IMAPPort:     993,
		IMAPTLS:      true,
		IMAPStartTLS: false,
		SMTPHost:     host,
		SMTPPort:     port,
		SMTPTLS:      false,
		SMTPStartTLS: false,
		IsDefault:    true,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	identity, err := st.CreateMailIdentity(context.Background(), models.MailIdentity{
		ID:          "ident-admin",
		AccountID:   account.ID,
		DisplayName: "Alias",
		FromEmail:   "alias@example.com",
		IsDefault:   true,
	})
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"to":          []string{"alice@example.com"},
		"subject":     "identity",
		"body":        "hello",
		"from_mode":   "identity",
		"identity_id": identity.ID,
	})
	rec := postSendJSON(t, router, sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	smtpCapture.wait(t)
	from, rcpt, _, authUser := smtpCapture.snapshot()
	if !strings.Contains(strings.ToLower(from), "alias@example.com") {
		t.Fatalf("expected smtp from to use identity sender, got %q", from)
	}
	if len(rcpt) != 1 || !strings.Contains(strings.ToLower(rcpt[0]), "alice@example.com") {
		t.Fatalf("unexpected recipients: %#v", rcpt)
	}
	if authUser != "mailbox@example.com" {
		t.Fatalf("expected smtp auth to use mailbox login, got %q", authUser)
	}
}

func TestSendIdentityModeRejectsForeignIdentity(t *testing.T) {
	despatch := &sendTestDespatch{}
	router, st, cfg := newSendRouterWithStore(t, despatch, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	pwHash, err := auth.HashPassword("SecretPass123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	other, err := st.CreateUser(context.Background(), "other@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	secretEnc, err := util.EncryptString(util.Derive32ByteKey(cfg.SessionEncryptKey), "mailbox-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	account, err := st.CreateMailAccount(context.Background(), models.MailAccount{
		ID:           "acct-other",
		UserID:       other.ID,
		DisplayName:  "Other",
		Login:        "other@example.com",
		SecretEnc:    secretEnc,
		IMAPHost:     "imap.example.com",
		IMAPPort:     993,
		IMAPTLS:      true,
		IMAPStartTLS: false,
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPTLS:      false,
		SMTPStartTLS: true,
		IsDefault:    true,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	identity, err := st.CreateMailIdentity(context.Background(), models.MailIdentity{
		ID:          "ident-other",
		AccountID:   account.ID,
		DisplayName: "Other",
		FromEmail:   "other@example.com",
		IsDefault:   true,
	})
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"to":          []string{"alice@example.com"},
		"subject":     "forbidden",
		"body":        "body",
		"from_mode":   "identity",
		"identity_id": identity.ID,
	})
	rec := postSendJSON(t, router, sessionCookie, csrfCookie, body)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	var apiErr util.APIError
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode api error: %v body=%s", err, rec.Body.String())
	}
	if apiErr.Code != "sender_identity_forbidden" {
		t.Fatalf("expected sender_identity_forbidden, got %q", apiErr.Code)
	}
}

type fakeSMTPServer struct {
	addr string

	mu       sync.Mutex
	from     string
	rcpt     []string
	data     string
	authUser string

	done chan struct{}
	ln   net.Listener
}

func startFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen smtp: %v", err)
	}
	srv := &fakeSMTPServer{
		addr: ln.Addr().String(),
		done: make(chan struct{}),
		ln:   ln,
	}
	go srv.serve()
	t.Cleanup(func() {
		_ = srv.ln.Close()
		select {
		case <-srv.done:
		case <-time.After(3 * time.Second):
			t.Fatalf("timeout waiting for smtp server shutdown")
		}
	})
	return srv
}

func (s *fakeSMTPServer) serve() {
	defer close(s.done)
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	writeLine := func(line string) bool {
		if _, err := rw.WriteString(line + "\r\n"); err != nil {
			return false
		}
		return rw.Flush() == nil
	}
	if !writeLine("220 fake-smtp ready") {
		return
	}

	for {
		line, err := rw.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.TrimSpace(line)
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "EHLO "), strings.HasPrefix(upper, "HELO "):
			if !writeLine("250-fake-smtp") {
				return
			}
			if !writeLine("250-AUTH PLAIN") {
				return
			}
			if !writeLine("250 OK") {
				return
			}
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			payload := strings.TrimSpace(cmd[len("AUTH PLAIN"):])
			if payload == "" {
				if !writeLine("334 ") {
					return
				}
				challenge, err := rw.ReadString('\n')
				if err != nil {
					return
				}
				payload = strings.TrimSpace(challenge)
			}
			decoded, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				if !writeLine("535 invalid auth") {
					return
				}
				return
			}
			parts := strings.Split(string(decoded), "\x00")
			if len(parts) < 3 {
				if !writeLine("535 invalid auth") {
					return
				}
				return
			}
			s.mu.Lock()
			s.authUser = parts[1]
			s.mu.Unlock()
			if !writeLine("235 2.7.0 Authentication successful") {
				return
			}
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.mu.Lock()
			s.from = strings.TrimSpace(cmd[len("MAIL FROM:"):])
			s.mu.Unlock()
			if !writeLine("250 OK") {
				return
			}
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.mu.Lock()
			s.rcpt = append(s.rcpt, strings.TrimSpace(cmd[len("RCPT TO:"):]))
			s.mu.Unlock()
			if !writeLine("250 OK") {
				return
			}
		case upper == "DATA":
			if !writeLine("354 End data with <CR><LF>.<CR><LF>") {
				return
			}
			var dataBuf strings.Builder
			for {
				dl, err := rw.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(dl) == "." {
					break
				}
				dataBuf.WriteString(dl)
			}
			s.mu.Lock()
			s.data = dataBuf.String()
			s.mu.Unlock()
			if !writeLine("250 OK") {
				return
			}
		case upper == "QUIT":
			_ = writeLine("221 Bye")
			return
		default:
			if !writeLine("250 OK") {
				return
			}
		}
	}
}

func (s *fakeSMTPServer) wait(t *testing.T) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatalf("smtp server did not finish")
	}
}

func (s *fakeSMTPServer) snapshot() (string, []string, string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	outRcpt := append([]string(nil), s.rcpt...)
	return s.from, outRcpt, s.data, s.authUser
}
