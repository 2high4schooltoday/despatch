package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/models"
)

func newOutboundTestStore(t *testing.T) *Store {
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
		filepath.Join("..", "..", "migrations", "030_mail_triage.sql"),
		filepath.Join("..", "..", "migrations", "031_reply_funnels.sql"),
		filepath.Join("..", "..", "migrations", "032_mail_account_providers.sql"),
		filepath.Join("..", "..", "migrations", "033_user_preferences_locale.sql"),
		filepath.Join("..", "..", "migrations", "034_user_preferences_format_locale.sql"),
		filepath.Join("..", "..", "migrations", "035_outbound_campaigns.sql"),
		filepath.Join("..", "..", "migrations", "036_outbound_campaign_steps.sql"),
		filepath.Join("..", "..", "migrations", "037_outbound_enrollments.sql"),
		filepath.Join("..", "..", "migrations", "038_outbound_events.sql"),
		filepath.Join("..", "..", "migrations", "039_recipient_state.sql"),
		filepath.Join("..", "..", "migrations", "040_mail_thread_bindings.sql"),
		filepath.Join("..", "..", "migrations", "041_outbound_suppressions.sql"),
		filepath.Join("..", "..", "migrations", "042_outbound_campaign_intelligence.sql"),
	} {
		if err := db.ApplyMigrationFile(sqdb, migration); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}
	return New(sqdb)
}

func createOutboundTestAccount(t *testing.T, st *Store, userID, accountID, login string) models.MailAccount {
	t.Helper()
	item, err := st.CreateMailAccount(context.Background(), models.MailAccount{
		ID:           accountID,
		UserID:       userID,
		DisplayName:  login,
		Login:        login,
		SecretEnc:    "enc",
		IMAPHost:     "127.0.0.1",
		IMAPPort:     993,
		IMAPTLS:      true,
		SMTPHost:     "127.0.0.1",
		SMTPPort:     587,
		SMTPStartTLS: true,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("create mail account %s: %v", accountID, err)
	}
	return item
}

func TestFindMailThreadBindingByReplyHeadersMatchesCollectorAccount(t *testing.T) {
	st := newOutboundTestStore(t)
	ctx := context.Background()

	user, err := st.CreateUser(ctx, "collector-test@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	source := createOutboundTestAccount(t, st, user.ID, "acct-source", "source@gmail.com")
	collector := createOutboundTestAccount(t, st, user.ID, "acct-collector", "collector@example.com")
	campaign, err := st.CreateOutboundCampaign(ctx, models.OutboundCampaign{
		UserID:             user.ID,
		Name:               "Collector Match",
		Status:             "draft",
		AudienceSourceKind: "manual",
		SenderPolicyKind:   "single_sender",
		SenderPolicyRef:    source.ID,
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	enrollment, err := st.UpsertOutboundEnrollment(ctx, models.OutboundEnrollment{
		CampaignID:      campaign.ID,
		RecipientEmail:  "lead@example.com",
		RecipientDomain: "example.com",
		SenderAccountID: source.ID,
		Status:          "waiting_reply",
	})
	if err != nil {
		t.Fatalf("create enrollment: %v", err)
	}

	binding, err := st.UpsertMailThreadBinding(ctx, models.MailThreadBinding{
		AccountID:             source.ID,
		ThreadID:              "thread-1",
		BindingType:           "outbound_enrollment",
		CampaignID:            campaign.ID,
		EnrollmentID:          enrollment.ID,
		ReplyAccountID:        collector.ID,
		CollectorAccountID:    collector.ID,
		OwnerUserID:           user.ID,
		RecipientEmail:        "lead@example.com",
		RecipientDomain:       "example.com",
		RootOutboundMessageID: "<root-1@example.com>",
		LastOutboundMessageID: "<root-1@example.com>",
	})
	if err != nil {
		t.Fatalf("upsert binding: %v", err)
	}

	found, err := st.FindMailThreadBindingByReplyHeaders(ctx, collector.ID, []string{mail.NormalizeMessageIDHeader("<root-1@example.com>")})
	if err != nil {
		t.Fatalf("find binding by collector reply headers: %v", err)
	}
	if found.ID != binding.ID {
		t.Fatalf("expected binding %q, got %q", binding.ID, found.ID)
	}
}
