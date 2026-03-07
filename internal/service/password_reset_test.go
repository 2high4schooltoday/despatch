package service

import (
	"context"
	"errors"
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

type captureResetSender struct {
	to    string
	token string
	calls int
	err   error
}

type testResetProvisioner struct{}

func (testResetProvisioner) UpsertActiveUser(ctx context.Context, email, passwordHash string) error {
	_ = ctx
	_ = email
	_ = passwordHash
	return nil
}

func (testResetProvisioner) DisableUser(ctx context.Context, email string) error {
	_ = ctx
	_ = email
	return nil
}

func (s *captureResetSender) SendPasswordReset(ctx context.Context, toEmail, token string) error {
	_ = ctx
	s.calls++
	s.to = toEmail
	s.token = token
	return s.err
}

func newPasswordResetService(t *testing.T, sender *captureResetSender) (*Service, *store.Store) {
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
	cfg := config.Config{
		DovecotAuthMode:              "sql",
		SessionEncryptKey:            "this_is_a_test_session_key_that_is_long_enough_123456",
		SessionIdleMinutes:           30,
		SessionAbsoluteHour:          24,
		PasswordMinLength:            12,
		PasswordMaxLength:            128,
		PasswordResetPublicEnabled:   true,
		PasswordResetTokenTTLMinutes: 30,
		PasswordResetSender:          "smtp",
	}
	svc := New(cfg, st, mail.NoopClient{}, testResetProvisioner{}, sender)
	return svc, st
}

func TestRequestPasswordResetSendsTokenToRecoveryEmail(t *testing.T) {
	ctx := context.Background()
	sender := &captureResetSender{}
	svc, st := newPasswordResetService(t, sender)

	pwHash, err := auth.HashPassword("ResetMe123!!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u, err := st.CreateUser(ctx, "account@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(ctx, u.ID, "recovery@example.net"); err != nil {
		t.Fatalf("update recovery email: %v", err)
	}

	if err := svc.RequestPasswordReset(ctx, "account@example.com"); err != nil {
		t.Fatalf("request reset: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one reset email send, got %d", sender.calls)
	}
	if sender.to != "recovery@example.net" {
		t.Fatalf("expected reset token delivery to recovery@example.net, got %q", sender.to)
	}
	if sender.token == "" {
		t.Fatalf("expected non-empty reset token")
	}
}

func TestRequestPasswordResetSkipsLegacyUserWithoutRecoveryEmail(t *testing.T) {
	ctx := context.Background()
	sender := &captureResetSender{}
	svc, st := newPasswordResetService(t, sender)

	pwHash, err := auth.HashPassword("LegacyUser123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u, err := st.CreateUser(ctx, "legacy@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(ctx, u.ID, ""); err != nil {
		t.Fatalf("clear recovery email: %v", err)
	}

	if err := svc.RequestPasswordReset(ctx, "legacy@example.com"); err != nil {
		t.Fatalf("request reset: %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("expected no reset email send for missing recovery email, got %d", sender.calls)
	}
}

func TestRequestPasswordResetReturnsSoftErrorAndInvalidatesTokenWhenDeliveryFails(t *testing.T) {
	ctx := context.Background()
	sender := &captureResetSender{err: errors.New("smtp delivery failed")}
	svc, st := newPasswordResetService(t, sender)

	pwHash, err := auth.HashPassword("DeliveryFail123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u, err := st.CreateUser(ctx, "delivery-fail@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(ctx, u.ID, "delivery-recovery@example.net"); err != nil {
		t.Fatalf("update recovery email: %v", err)
	}

	err = svc.RequestPasswordReset(ctx, "delivery-fail@example.com")
	if !errors.Is(err, ErrPasswordResetDelivery) {
		t.Fatalf("expected ErrPasswordResetDelivery, got %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one reset delivery attempt, got %d", sender.calls)
	}
	if sender.token == "" {
		t.Fatalf("expected generated reset token on failed send")
	}
	confirmErr := svc.ConfirmPasswordReset(ctx, sender.token, "ReplacementPass123!")
	if !errors.Is(confirmErr, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials for invalidated token, got %v", confirmErr)
	}
}

func TestRequestPasswordResetAcceptsRecoveryEmailIdentifier(t *testing.T) {
	ctx := context.Background()
	sender := &captureResetSender{}
	svc, st := newPasswordResetService(t, sender)

	pwHash, err := auth.HashPassword("RecoveryIdentifier123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u, err := st.CreateUser(ctx, "account2@example.com", pwHash, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(ctx, u.ID, "recover-lookup@example.net"); err != nil {
		t.Fatalf("update recovery email: %v", err)
	}

	if err := svc.RequestPasswordReset(ctx, "recover-lookup@example.net"); err != nil {
		t.Fatalf("request reset by recovery email: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("expected one reset email send, got %d", sender.calls)
	}
	if sender.to != "recover-lookup@example.net" {
		t.Fatalf("expected reset token delivery to recover-lookup@example.net, got %q", sender.to)
	}
}

func TestRequestPasswordResetSkipsAmbiguousRecoveryEmailIdentifier(t *testing.T) {
	ctx := context.Background()
	sender := &captureResetSender{}
	svc, st := newPasswordResetService(t, sender)

	pwHash1, err := auth.HashPassword("AmbiguousReset123!")
	if err != nil {
		t.Fatalf("hash password 1: %v", err)
	}
	u1, err := st.CreateUser(ctx, "ambiguous-1@example.com", pwHash1, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user 1: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(ctx, u1.ID, "shared-recovery@example.net"); err != nil {
		t.Fatalf("set user 1 recovery email: %v", err)
	}

	pwHash2, err := auth.HashPassword("AmbiguousReset124!")
	if err != nil {
		t.Fatalf("hash password 2: %v", err)
	}
	u2, err := st.CreateUser(ctx, "ambiguous-2@example.com", pwHash2, "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user 2: %v", err)
	}
	if err := st.UpdateUserRecoveryEmail(ctx, u2.ID, "shared-recovery@example.net"); err != nil {
		t.Fatalf("set user 2 recovery email: %v", err)
	}

	if err := svc.RequestPasswordReset(ctx, "shared-recovery@example.net"); err != nil {
		t.Fatalf("request reset by shared recovery email: %v", err)
	}
	if sender.calls != 0 {
		t.Fatalf("expected no reset send for ambiguous recovery email, got %d", sender.calls)
	}
}
