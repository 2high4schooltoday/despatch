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
		PasswordResetSender:          "log",
	}
	svc := New(cfg, st, mail.NoopClient{}, mail.NoopProvisioner{}, sender)
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
