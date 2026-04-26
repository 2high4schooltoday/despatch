package service

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/models"
	"despatch/internal/store"
)

type pamTestDespatch struct {
	acceptPassword string
	acceptedUsers  map[string]bool
	failWith       error
}

type fakePAMResetter struct {
	err       error
	calls     int
	usernames []string
}

type probeFailResetSender struct {
	captureResetSender
	probeErr error
}

func (f *fakePAMResetter) ResetPassword(ctx context.Context, username, newPassword string) error {
	_ = ctx
	_ = newPassword
	f.calls++
	f.usernames = append(f.usernames, username)
	return f.err
}

func (s *probeFailResetSender) ProbePasswordReset(ctx context.Context) error {
	_ = ctx
	return s.probeErr
}

func (m pamTestDespatch) ListMailboxes(ctx context.Context, user, pass string) ([]mail.Mailbox, error) {
	if m.failWith != nil {
		return nil, m.failWith
	}
	if pass != m.acceptPassword {
		return nil, errors.New("imap auth failed")
	}
	if len(m.acceptedUsers) > 0 && !m.acceptedUsers[user] {
		return nil, errors.New("imap auth failed")
	}
	return []mail.Mailbox{{Name: "INBOX", Messages: 1}}, nil
}

func (m pamTestDespatch) CreateMailbox(ctx context.Context, user, pass, mailbox string) error {
	return nil
}
func (m pamTestDespatch) RenameMailbox(ctx context.Context, user, pass, mailbox, newMailbox string) error {
	return nil
}
func (m pamTestDespatch) DeleteMailbox(ctx context.Context, user, pass, mailbox string) error {
	return nil
}
func (m pamTestDespatch) ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]mail.MessageSummary, error) {
	return nil, nil
}
func (m pamTestDespatch) GetMessage(ctx context.Context, user, pass, id string) (mail.Message, error) {
	return mail.Message{}, nil
}
func (m pamTestDespatch) GetRawMessage(ctx context.Context, user, pass, id string) ([]byte, error) {
	return []byte(""), nil
}
func (m pamTestDespatch) Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]mail.MessageSummary, error) {
	return nil, nil
}
func (m pamTestDespatch) Send(ctx context.Context, user, pass string, req mail.SendRequest) (mail.SendResult, error) {
	return mail.SendResult{SavedCopy: true, SavedCopyMailbox: "Sent"}, nil
}
func (m pamTestDespatch) SetFlags(ctx context.Context, user, pass, id string, flags []string) error {
	return nil
}
func (m pamTestDespatch) UpdateFlags(ctx context.Context, user, pass, id string, patch mail.FlagPatch) error {
	return nil
}
func (m pamTestDespatch) Move(ctx context.Context, user, pass, id, mailbox string) error {
	return nil
}
func (m pamTestDespatch) GetAttachment(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentContent, error) {
	return mail.AttachmentContent{}, nil
}
func (m pamTestDespatch) GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentMeta, io.ReadCloser, error) {
	return mail.AttachmentMeta{}, nil, errors.New("not implemented")
}

func newPAMTestService(t *testing.T, acceptedPass string, acceptedUsers ...string) (*Service, *store.Store) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "app.db")
	sqdb, err := db.OpenSQLite(tmp, 2, 1, 5*time.Minute)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqdb.Close() })
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "001_init.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "002_users_mail_login.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "003_cleanup_rejected_users.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "004_cleanup_rejected_users_casefold.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "005_admin_query_indexes.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "006_users_recovery_email.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "007_mail_accounts.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "032_mail_account_providers.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "021_password_reset_token_reservations.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	st := store.New(sqdb)
	cfg := config.Config{
		DovecotAuthMode:                 "pam",
		SessionEncryptKey:               "this_is_a_test_session_key_that_is_long_enough_123456",
		SessionIdleMinutes:              30,
		SessionAbsoluteHour:             24,
		PasswordMinLength:               12,
		PasswordMaxLength:               128,
		PasswordResetPublicEnabled:      true,
		PasswordResetTokenTTLMinutes:    30,
		PasswordResetRequireMappedLogin: true,
		IMAPHost:                        "127.0.0.1",
		IMAPPort:                        993,
		IMAPTLS:                         true,
		SMTPHost:                        "127.0.0.1",
		SMTPPort:                        25,
		SMTPTLS:                         false,
		SMTPStartTLS:                    false,
	}
	accepted := map[string]bool{}
	for _, v := range acceptedUsers {
		accepted[v] = true
	}
	svc := New(cfg, st, pamTestDespatch{acceptPassword: acceptedPass, acceptedUsers: accepted}, mail.NoopProvisioner{}, nil)
	return svc, st
}

func newSQLLocalTestService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "app.db")
	sqdb, err := db.OpenSQLite(tmp, 2, 1, 5*time.Minute)
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
		filepath.Join("..", "..", "migrations", "019_users_mail_secret.sql"),
		filepath.Join("..", "..", "migrations", "032_mail_account_providers.sql"),
	} {
		if err := db.ApplyMigrationFile(sqdb, migration); err != nil {
			t.Fatalf("apply migrations: %v", err)
		}
	}

	st := store.New(sqdb)
	cfg := config.Config{
		DovecotAuthMode:     "sql",
		SessionEncryptKey:   "this_is_a_test_session_key_that_is_long_enough_123456",
		SessionIdleMinutes:  30,
		SessionAbsoluteHour: 24,
		PasswordMinLength:   12,
		PasswordMaxLength:   128,
		IMAPHost:            "127.0.0.1",
		IMAPPort:            993,
		IMAPTLS:             true,
		SMTPHost:            "127.0.0.1",
		SMTPPort:            25,
		SMTPTLS:             false,
		SMTPStartTLS:        false,
	}
	svc := New(cfg, st, mail.NoopClient{}, mail.NoopProvisioner{}, nil)
	return svc, st
}

func TestPAMModeLoginUsesMailCredentials(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!")

	localHash, err := auth.HashPassword("LocalOnly123!")
	if err != nil {
		t.Fatalf("hash local: %v", err)
	}
	u, err := st.CreateUser(ctx, "alice@example.com", localHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateProvisionState(ctx, u.ID, "ok", nil); err != nil {
		t.Fatalf("set provision state: %v", err)
	}

	if _, _, err := svc.Login(ctx, "alice@example.com", "LocalOnly123!", "127.0.0.1", "test-agent"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials for local hash password, got: %v", err)
	}
	if _, _, err := svc.Login(ctx, "alice@example.com", "PamPass123!", "127.0.0.1", "test-agent"); err != nil {
		t.Fatalf("expected PAM-backed login success, got: %v", err)
	}
}

func TestPAMModeLoginBootstrapsIndexedAccount(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!", "alice")

	localHash, err := auth.HashPassword("LocalOnly123!")
	if err != nil {
		t.Fatalf("hash local: %v", err)
	}
	u, err := st.CreateUser(ctx, "alice@example.com", localHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateProvisionState(ctx, u.ID, "ok", nil); err != nil {
		t.Fatalf("set provision state: %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, _, err := svc.Login(ctx, "alice@example.com", "PamPass123!", "127.0.0.1", "test-agent"); err != nil {
			t.Fatalf("login %d failed: %v", i+1, err)
		}
	}

	accounts, err := st.ListMailAccounts(ctx, u.ID)
	if err != nil {
		t.Fatalf("list mail accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 indexed account, got %d", len(accounts))
	}
	account := accounts[0]
	if account.Login != "alice" {
		t.Fatalf("expected account login alice, got %q", account.Login)
	}
	if !account.IsDefault {
		t.Fatalf("expected bootstrapped account to be default")
	}
	if account.IMAPHost != "127.0.0.1" || account.IMAPPort != 993 {
		t.Fatalf("unexpected IMAP settings: %s:%d", account.IMAPHost, account.IMAPPort)
	}
	if account.SMTPHost != "127.0.0.1" || account.SMTPPort != 25 {
		t.Fatalf("unexpected SMTP settings: %s:%d", account.SMTPHost, account.SMTPPort)
	}
	if account.SecretEnc == "" {
		t.Fatalf("expected bootstrapped account to persist encrypted secret")
	}
}

func TestSQLEnsureAuthenticatedMailAccountBootstrapsIndexedAccountForLocalInstance(t *testing.T) {
	ctx := context.Background()
	svc, st := newSQLLocalTestService(t)

	pwHash, err := auth.HashPassword("LocalPass123!")
	if err != nil {
		t.Fatalf("hash local: %v", err)
	}
	u, err := st.CreateUser(ctx, "alice@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, _, err := svc.Login(ctx, "alice@example.com", "LocalPass123!", "127.0.0.1", "test-agent"); err != nil {
		t.Fatalf("login failed: %v", err)
	}
	user, err := st.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if err := svc.EnsureAuthenticatedMailAccount(ctx, user); err != nil {
		t.Fatalf("ensure authenticated mail account: %v", err)
	}

	accounts, err := st.ListMailAccounts(ctx, u.ID)
	if err != nil {
		t.Fatalf("list mail accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 indexed account, got %d", len(accounts))
	}
	account := accounts[0]
	if account.Login != "alice@example.com" {
		t.Fatalf("expected account login alice@example.com, got %q", account.Login)
	}
	if !account.IsDefault {
		t.Fatalf("expected bootstrapped account to be default")
	}
	if account.IMAPHost != "127.0.0.1" || account.IMAPPort != 993 {
		t.Fatalf("unexpected IMAP settings: %s:%d", account.IMAPHost, account.IMAPPort)
	}
	if account.SMTPHost != "127.0.0.1" || account.SMTPPort != 25 {
		t.Fatalf("unexpected SMTP settings: %s:%d", account.SMTPHost, account.SMTPPort)
	}
	if account.SecretEnc == "" {
		t.Fatalf("expected bootstrapped account to persist encrypted secret")
	}
}

func TestSQLEnsureAuthenticatedMailAccountDoesNotBootstrapIndexedAccountForExternalInstance(t *testing.T) {
	ctx := context.Background()
	svc, st := newSQLLocalTestService(t)
	if err := st.UpsertSetting(ctx, instanceModeSettingKey, InstanceModeExternalAccounts); err != nil {
		t.Fatalf("set instance mode: %v", err)
	}

	pwHash, err := auth.HashPassword("LocalPass123!")
	if err != nil {
		t.Fatalf("hash local: %v", err)
	}
	u, err := st.CreateUser(ctx, "alice@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, _, err := svc.Login(ctx, "alice@example.com", "LocalPass123!", "127.0.0.1", "test-agent"); err != nil {
		t.Fatalf("login failed: %v", err)
	}
	user, err := st.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if err := svc.EnsureAuthenticatedMailAccount(ctx, user); err != nil {
		t.Fatalf("ensure authenticated mail account: %v", err)
	}

	accounts, err := st.ListMailAccounts(ctx, u.ID)
	if err != nil {
		t.Fatalf("list mail accounts: %v", err)
	}
	if len(accounts) != 0 {
		t.Fatalf("expected no indexed accounts in external mode, got %+v", accounts)
	}
}

func TestPAMUnlockBootstrapsIndexedAccountUsingAcceptedLogin(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!", "alice")

	localHash, err := auth.HashPassword("LocalOnly123!")
	if err != nil {
		t.Fatalf("hash local: %v", err)
	}
	u, err := st.CreateUser(ctx, "alice@example.com", localHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now().UTC()
	if err := st.CreateSession(ctx, models.Session{
		ID:            "session-1",
		UserID:        u.ID,
		TokenHash:     "token-hash-1",
		AuthMethod:    "password",
		ExpiresAt:     now.Add(time.Hour),
		IdleExpiresAt: now.Add(time.Hour),
		CreatedAt:     now,
		LastSeenAt:    now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := svc.UnlockSessionMailSecret(ctx, u.ID, "session-1", "PamPass123!"); err != nil {
		t.Fatalf("unlock mail secret: %v", err)
	}

	accounts, err := st.ListMailAccounts(ctx, u.ID)
	if err != nil {
		t.Fatalf("list mail accounts: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 indexed account, got %d", len(accounts))
	}
	if accounts[0].Login != "alice" {
		t.Fatalf("expected unlock bootstrap login alice, got %q", accounts[0].Login)
	}
}

func TestPAMModeLoginDetectsConnectivityErrors(t *testing.T) {
	ctx := context.Background()
	tmp := filepath.Join(t.TempDir(), "app.db")
	sqdb, err := db.OpenSQLite(tmp, 2, 1, 5*time.Minute)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqdb.Close() })
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "001_init.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "002_users_mail_login.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "003_cleanup_rejected_users.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "004_cleanup_rejected_users_casefold.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "005_admin_query_indexes.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "006_users_recovery_email.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "021_password_reset_token_reservations.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	st := store.New(sqdb)
	cfg := config.Config{
		DovecotAuthMode:     "pam",
		SessionEncryptKey:   "this_is_a_test_session_key_that_is_long_enough_123456",
		SessionIdleMinutes:  30,
		SessionAbsoluteHour: 24,
	}
	svc := New(cfg, st, pamTestDespatch{failWith: errors.New("dial tcp 127.0.0.1:993: connect: connection refused")}, mail.NoopProvisioner{}, nil)

	localHash, err := auth.HashPassword("AnyPassword123!")
	if err != nil {
		t.Fatalf("hash local: %v", err)
	}
	u, err := st.CreateUser(ctx, "alice@example.com", localHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateProvisionState(ctx, u.ID, "ok", nil); err != nil {
		t.Fatalf("set provision state: %v", err)
	}

	if _, _, err := svc.Login(ctx, "alice@example.com", "AnyPassword123!", "127.0.0.1", "test-agent"); !errors.Is(err, ErrPAMVerifierDown) {
		t.Fatalf("expected ErrPAMVerifierDown, got: %v", err)
	}
}

func TestPAMModeUsesHelperPolicyForPasswordReset(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!")

	pwHash, err := auth.HashPassword("AnyPassword123!")
	if err != nil {
		t.Fatalf("hash local: %v", err)
	}
	user, err := st.CreateUser(ctx, "alice@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Public request path is unavailable when helper is disabled.
	if err := svc.RequestPasswordReset(ctx, "alice@example.com"); !errors.Is(err, ErrPasswordResetUnavailable) {
		t.Fatalf("expected RequestPasswordReset to be unavailable, got: %v", err)
	}
	if err := svc.ConfirmPasswordReset(ctx, "token", "NewPassword123!"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ConfirmPasswordReset invalid token, got: %v", err)
	}
	if err := svc.AdminResetPassword(ctx, "admin-id", user.ID, "NewPassword123!"); !errors.Is(err, ErrPasswordResetHelperDown) {
		t.Fatalf("expected AdminResetPassword helper unavailable, got: %v", err)
	}
}

func TestPAMPublicResetRequiresConfirmedExternalSender(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!")
	svc.cfg.PAMResetHelperEnabled = true
	svc.sender = &captureResetSender{}

	pwHash, err := auth.HashPassword("PamResetUser123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(ctx, "pam-reset@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(ctx, user.ID, "pam-reset-recovery@example.net"); err != nil {
		t.Fatalf("set recovery email: %v", err)
	}

	if err := svc.RequestPasswordReset(ctx, "pam-reset@example.com"); !errors.Is(err, ErrPasswordResetUnavailable) {
		t.Fatalf("expected public reset unavailable until external sender is confirmed, got %v", err)
	}
	caps := svc.PasswordResetCapabilities(ctx)
	if caps.SelfServiceEnabled {
		t.Fatalf("expected self-service reset disabled, got %+v", caps)
	}
	if caps.SenderReason != passwordResetSenderReasonExternalUnconfirmed {
		t.Fatalf("expected external sender unconfirmed reason, got %+v", caps)
	}
}

func TestPAMPublicResetAllowsConfirmedExternalSender(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!")
	svc.cfg.PAMResetHelperEnabled = true
	svc.cfg.PasswordResetExternalSenderReady = true
	sender := &captureResetSender{}
	svc.sender = sender

	pwHash, err := auth.HashPassword("PamResetUser123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(ctx, "pam-confirmed@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(ctx, user.ID, "pam-confirmed-recovery@example.net"); err != nil {
		t.Fatalf("set recovery email: %v", err)
	}

	if err := svc.RequestPasswordReset(ctx, "pam-confirmed@example.com"); err != nil {
		t.Fatalf("expected confirmed external sender reset request to succeed, got %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one delivery attempt, got %d", sender.calls)
	}
	caps := svc.PasswordResetCapabilities(ctx)
	if !caps.SelfServiceEnabled || caps.SenderReason != "" {
		t.Fatalf("unexpected capabilities after external sender confirmation: %+v", caps)
	}
}

func TestPAMPublicResetRequiresHealthySMTPProbe(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!")
	svc.cfg.PAMResetHelperEnabled = true
	svc.cfg.PasswordResetExternalSenderReady = true
	sender := &probeFailResetSender{probeErr: errors.New("dial tcp 127.0.0.1:587: connect: connection refused")}
	svc.sender = sender

	pwHash, err := auth.HashPassword("PamResetUser124!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(ctx, "pam-probe@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(ctx, user.ID, "pam-probe-recovery@example.net"); err != nil {
		t.Fatalf("set recovery email: %v", err)
	}

	if err := svc.RequestPasswordReset(ctx, "pam-probe@example.com"); !errors.Is(err, ErrPasswordResetUnavailable) {
		t.Fatalf("expected public reset unavailable with broken smtp probe, got %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("expected no reset delivery attempt when smtp probe fails, got %d", sender.calls)
	}
	caps := svc.PasswordResetCapabilities(ctx)
	if caps.SelfServiceEnabled || caps.SenderStatus != passwordResetSenderStatusDegraded || caps.SenderReason != passwordResetSenderReasonSMTPUnreachable {
		t.Fatalf("unexpected capabilities for failed smtp probe: %+v", caps)
	}
}

func TestPAMConfirmPasswordResetKeepsTokenWhenHelperUnavailable(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!")
	svc.cfg.PAMResetHelperEnabled = true
	resetter := &fakePAMResetter{err: ErrPasswordResetHelperDown}
	svc.pamResetter = resetter

	pwHash, err := auth.HashPassword("PamToken123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user, err := st.CreateUser(ctx, "pam-token@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessionTokenHash := "session-token-hash"
	now := time.Now().UTC()
	if err := st.CreateSession(ctx, models.Session{
		ID:            "sess-1",
		UserID:        user.ID,
		TokenHash:     sessionTokenHash,
		MailSecret:    "",
		ExpiresAt:     now.Add(time.Hour),
		IdleExpiresAt: now.Add(time.Hour),
		CreatedAt:     now,
		LastSeenAt:    now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	rawToken, tokenHash, err := auth.NewOpaqueToken()
	if err != nil {
		t.Fatalf("new opaque token: %v", err)
	}
	if _, err := st.CreatePasswordResetToken(ctx, user.ID, tokenHash, time.Now().UTC().Add(5*time.Minute)); err != nil {
		t.Fatalf("create reset token: %v", err)
	}

	if err := svc.ConfirmPasswordReset(ctx, rawToken, "NewPassword123!"); !errors.Is(err, ErrPasswordResetHelperDown) {
		t.Fatalf("expected helper unavailable on first confirm, got %v", err)
	}
	resetter.err = nil
	if err := svc.ConfirmPasswordReset(ctx, rawToken, "NewPassword123!"); err != nil {
		t.Fatalf("expected second confirm to succeed after helper recovery, got %v", err)
	}
	if resetter.calls != 2 {
		t.Fatalf("expected helper to be called twice, got %d", resetter.calls)
	}
	if _, err := svc.st.ConsumePasswordResetToken(ctx, tokenHash); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected reset token to be used after successful retry, got %v", err)
	}
	session, err := st.GetSessionByTokenHash(ctx, sessionTokenHash)
	if err != nil {
		t.Fatalf("load session after reset: %v", err)
	}
	if session.RevokedAt == nil {
		t.Fatalf("expected user sessions to be revoked after successful reset")
	}
}

func TestPAMAdminResetFallsBackToEmailWhenMappedLoginMissing(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!")
	svc.cfg.PAMResetHelperEnabled = true
	svc.cfg.PasswordResetRequireMappedLogin = true

	pwHash, err := auth.HashPassword("AnyPassword123!")
	if err != nil {
		t.Fatalf("hash local: %v", err)
	}
	user, err := st.CreateUser(ctx, "unmapped@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := svc.AdminResetPassword(ctx, "admin-id", user.ID, "NewPassword123!"); !errors.Is(err, ErrPasswordResetHelperDown) {
		t.Fatalf("expected helper unavailable after fallback to email login, got: %v", err)
	}
}

func TestPAMSetupDetectsConnectivityErrors(t *testing.T) {
	ctx := context.Background()
	tmp := filepath.Join(t.TempDir(), "app.db")
	sqdb, err := db.OpenSQLite(tmp, 2, 1, 5*time.Minute)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqdb.Close() })
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "001_init.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "002_users_mail_login.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "003_cleanup_rejected_users.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "004_cleanup_rejected_users_casefold.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "005_admin_query_indexes.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "006_users_recovery_email.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := db.ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "021_password_reset_token_reservations.sql")); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	st := store.New(sqdb)
	cfg := config.Config{
		DovecotAuthMode:     "pam",
		SessionEncryptKey:   "this_is_a_test_session_key_that_is_long_enough_123456",
		SessionIdleMinutes:  30,
		SessionAbsoluteHour: 24,
		BaseDomain:          "example.com",
	}
	svc := New(cfg, st, pamTestDespatch{failWith: errors.New("dial tcp 127.0.0.1:993: connect: connection refused")}, mail.NoopProvisioner{}, nil)

	_, _, err = svc.CompleteSetup(ctx, SetupCompleteRequest{
		BaseDomain:         "example.com",
		AdminEmail:         "webmaster@example.com",
		AdminRecoveryEmail: "recovery@example.net",
		AdminPassword:      "AnyPassword123!",
		Region:             "us-east",
	}, "127.0.0.1", "agent")
	if !errors.Is(err, ErrPAMVerifierDown) {
		t.Fatalf("expected ErrPAMVerifierDown, got: %v", err)
	}
}

func TestPAMSetupFallsBackToLocalPartAndPersistsLogin(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!", "webmaster")

	_, user, err := svc.CompleteSetup(ctx, SetupCompleteRequest{
		BaseDomain:         "example.com",
		AdminEmail:         "webmaster@example.com",
		AdminRecoveryEmail: "recovery@example.net",
		AdminPassword:      "PamPass123!",
		Region:             "us-east",
	}, "127.0.0.1", "agent")
	if err != nil {
		t.Fatalf("expected setup success with localpart fallback, got: %v", err)
	}
	if user.MailLogin == nil || *user.MailLogin != "webmaster" {
		t.Fatalf("expected mail_login to be webmaster, got: %#v", user.MailLogin)
	}

	dbUser, err := st.GetUserByEmail(ctx, "webmaster@example.com")
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if dbUser.MailLogin == nil || *dbUser.MailLogin != "webmaster" {
		t.Fatalf("expected persisted mail_login webmaster, got: %#v", dbUser.MailLogin)
	}
}

func TestPAMSetupAcceptsExplicitMailboxLoginOverride(t *testing.T) {
	ctx := context.Background()
	svc, st := newPAMTestService(t, "PamPass123!", "custom_login")

	_, user, err := svc.CompleteSetup(ctx, SetupCompleteRequest{
		BaseDomain:         "example.com",
		AdminEmail:         "webmaster@example.com",
		AdminRecoveryEmail: "recovery@example.net",
		AdminMailboxLogin:  "custom_login",
		AdminPassword:      "PamPass123!",
		Region:             "us-east",
	}, "127.0.0.1", "agent")
	if err != nil {
		t.Fatalf("expected setup success with explicit login override, got: %v", err)
	}
	if user.MailLogin == nil || *user.MailLogin != "custom_login" {
		t.Fatalf("expected mail_login custom_login, got: %#v", user.MailLogin)
	}

	dbUser, err := st.GetUserByEmail(ctx, "webmaster@example.com")
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if dbUser.MailLogin == nil || *dbUser.MailLogin != "custom_login" {
		t.Fatalf("expected persisted custom mail_login, got: %#v", dbUser.MailLogin)
	}
}

func TestPAMSetupReturnsIdentityErrorWhenNoCandidateAuthenticates(t *testing.T) {
	ctx := context.Background()
	svc, _ := newPAMTestService(t, "PamPass123!", "other_login")

	_, _, err := svc.CompleteSetup(ctx, SetupCompleteRequest{
		BaseDomain:         "example.com",
		AdminEmail:         "webmaster@example.com",
		AdminRecoveryEmail: "recovery@example.net",
		AdminPassword:      "PamPass123!",
		Region:             "us-east",
	}, "127.0.0.1", "agent")
	if err == nil {
		t.Fatalf("expected PAM identity error, got nil")
	}
	var pamErr *PAMCredentialsInvalidError
	if !errors.As(err, &pamErr) {
		t.Fatalf("expected PAMCredentialsInvalidError, got: %T %v", err, err)
	}
	if len(pamErr.Attempts) != 2 || pamErr.Attempts[0] != "webmaster@example.com" || pamErr.Attempts[1] != "webmaster" {
		t.Fatalf("unexpected attempts list: %#v", pamErr.Attempts)
	}
}

func TestMailIdentityUsesPrimaryEmailWhileMailAuthLoginPrefersStoredLogin(t *testing.T) {
	u := models.User{Email: "webmaster@example.com"}
	if got := MailIdentity(u); got != "webmaster@example.com" {
		t.Fatalf("expected sender identity email, got %q", got)
	}
	if got := MailAuthLogin(u); got != "webmaster@example.com" {
		t.Fatalf("expected auth login fallback email, got %q", got)
	}

	stored := "webmaster"
	u.MailLogin = &stored
	if got := MailIdentity(u); got != "webmaster@example.com" {
		t.Fatalf("expected sender identity to remain primary email, got %q", got)
	}
	if got := MailAuthLogin(u); got != "webmaster" {
		t.Fatalf("expected stored mail_login auth login, got %q", got)
	}
}
