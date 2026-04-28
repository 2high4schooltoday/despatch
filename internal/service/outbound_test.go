package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/models"
	"despatch/internal/store"
)

func newOutboundTestService(t *testing.T) (*Service, *store.Store, *sql.DB) {
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
		SMTPPort:            587,
		SMTPStartTLS:        true,
	}
	svc := New(cfg, st, nil, nil, nil)
	return svc, st, sqdb
}

func createOutboundServiceAccount(t *testing.T, st *store.Store, userID, accountID, login, providerType string) models.MailAccount {
	t.Helper()
	item, err := st.CreateMailAccount(context.Background(), models.MailAccount{
		ID:             accountID,
		UserID:         userID,
		DisplayName:    login,
		Login:          login,
		SecretEnc:      "enc",
		IMAPHost:       "127.0.0.1",
		IMAPPort:       993,
		IMAPTLS:        true,
		SMTPHost:       "127.0.0.1",
		SMTPPort:       587,
		SMTPStartTLS:   true,
		ProviderType:   providerType,
		ProviderLabel:  providerType,
		ConnectionMode: "imap_smtp",
		Status:         "active",
	})
	if err != nil {
		t.Fatalf("create account %s: %v", accountID, err)
	}
	return item
}

func createOutboundServiceIdentity(t *testing.T, st *store.Store, accountID, displayName, fromEmail, replyTo string) models.MailIdentity {
	t.Helper()
	item, err := st.CreateMailIdentity(context.Background(), models.MailIdentity{
		AccountID:   accountID,
		DisplayName: displayName,
		FromEmail:   fromEmail,
		ReplyTo:     replyTo,
		IsDefault:   true,
	})
	if err != nil {
		t.Fatalf("create identity for %s: %v", accountID, err)
	}
	return item
}

func TestOutboundSelectionFromReplyFunnelCollectorUsesCollectorReplyAccount(t *testing.T) {
	ctx := context.Background()
	svc, st, _ := newOutboundTestService(t)

	user, err := st.CreateUser(ctx, "ops@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	source := createOutboundServiceAccount(t, st, user.ID, "acct-source", "source@gmail.com", MailProviderTypeGmail)
	collector := createOutboundServiceAccount(t, st, user.ID, "acct-collector", "collector@libero.it", MailProviderTypeLibero)
	sourceIdentity := createOutboundServiceIdentity(t, st, source.ID, "Source Sender", source.Login, "")
	createOutboundServiceIdentity(t, st, collector.ID, "Collector Inbox", collector.Login, "replies@collector.example.com")

	funnel, err := st.CreateReplyFunnel(ctx, models.ReplyFunnel{
		UserID:             user.ID,
		Name:               "External Collector",
		SenderName:         "Outbound Team",
		CollectorAccountID: collector.ID,
		ReplyMode:          "collector",
		RoutingMode:        "virtual_inbox",
		IncludeCollector:   true,
		TargetReplyCount:   100,
	})
	if err != nil {
		t.Fatalf("create funnel: %v", err)
	}
	if _, err := st.UpsertReplyFunnelAccount(ctx, models.ReplyFunnelAccount{
		FunnelID:         funnel.ID,
		AccountID:        source.ID,
		Role:             "source",
		Position:         1,
		SenderIdentityID: sourceIdentity.ID,
	}); err != nil {
		t.Fatalf("upsert source funnel account: %v", err)
	}

	selection, err := svc.outboundSelectionFromReplyFunnel(ctx, user, funnel.ID, "lead@example.com")
	if err != nil {
		t.Fatalf("outboundSelectionFromReplyFunnel: %v", err)
	}
	if selection.Account.ID != source.ID {
		t.Fatalf("expected source account %q, got %q", source.ID, selection.Account.ID)
	}
	if selection.ReplyAccountID != collector.ID {
		t.Fatalf("expected reply account %q, got %q", collector.ID, selection.ReplyAccountID)
	}
	if selection.CollectorAccountID != collector.ID {
		t.Fatalf("expected collector account %q, got %q", collector.ID, selection.CollectorAccountID)
	}
	if selection.ReplyTo != "replies@collector.example.com" {
		t.Fatalf("expected collector reply-to to win, got %q", selection.ReplyTo)
	}
}

func TestApplyManualReplyOutcomeLoadsReplyFromCollectorAccount(t *testing.T) {
	ctx := context.Background()
	svc, st, sqdb := newOutboundTestService(t)

	user, err := st.CreateUser(ctx, "reviewer@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	source := createOutboundServiceAccount(t, st, user.ID, "acct-source", "source@gmail.com", MailProviderTypeGmail)
	collector := createOutboundServiceAccount(t, st, user.ID, "acct-collector", "collector@libero.it", MailProviderTypeLibero)
	createOutboundServiceIdentity(t, st, source.ID, "Source Sender", source.Login, "")
	createOutboundServiceIdentity(t, st, collector.ID, "Collector Inbox", collector.Login, "replies@collector.example.com")

	funnel, err := st.CreateReplyFunnel(ctx, models.ReplyFunnel{
		UserID:             user.ID,
		Name:               "Collector Flow",
		SenderName:         "Ops",
		CollectorAccountID: collector.ID,
		ReplyMode:          "collector",
		RoutingMode:        "virtual_inbox",
		IncludeCollector:   true,
		TargetReplyCount:   100,
	})
	if err != nil {
		t.Fatalf("create funnel: %v", err)
	}

	campaign, err := st.CreateOutboundCampaign(ctx, models.OutboundCampaign{
		UserID:                user.ID,
		Name:                  "Collector Campaign",
		Status:                "draft",
		AudienceSourceKind:    "manual",
		SenderPolicyKind:      "reply_funnel",
		SenderPolicyRef:       funnel.ID,
		ReplyPolicyJSON:       "{}",
		SuppressionPolicyJSON: "{}",
		SchedulePolicyJSON:    "{}",
		CompliancePolicyJSON:  "{}",
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	enrollment, err := st.UpsertOutboundEnrollment(ctx, models.OutboundEnrollment{
		CampaignID:      campaign.ID,
		RecipientEmail:  "lead@example.com",
		RecipientDomain: "example.com",
		SenderAccountID: source.ID,
		ReplyFunnelID:   funnel.ID,
		Status:          "waiting_reply",
	})
	if err != nil {
		t.Fatalf("create enrollment: %v", err)
	}
	if _, err := st.UpsertMailThreadBinding(ctx, models.MailThreadBinding{
		AccountID:             source.ID,
		ThreadID:              "",
		BindingType:           "outbound_enrollment",
		CampaignID:            campaign.ID,
		EnrollmentID:          enrollment.ID,
		FunnelID:              funnel.ID,
		ReplyAccountID:        collector.ID,
		CollectorAccountID:    collector.ID,
		OwnerUserID:           user.ID,
		RecipientEmail:        enrollment.RecipientEmail,
		RecipientDomain:       enrollment.RecipientDomain,
		RootOutboundMessageID: "<root-1@example.com>",
		LastOutboundMessageID: "<root-1@example.com>",
		LastReplyMessageID:    "reply-1",
	}); err != nil {
		t.Fatalf("upsert binding: %v", err)
	}

	now := time.Now().UTC().Round(time.Second)
	if _, err := sqdb.ExecContext(ctx,
		`INSERT INTO thread_index(
			id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"collector-thread", collector.ID, "INBOX", "re hello", "[]", 1, 0, 0, 0, 0, "reply-1", now, now,
	); err != nil {
		t.Fatalf("insert thread index: %v", err)
	}
	if _, err := sqdb.ExecContext(ctx,
		`INSERT INTO message_index(
			id,account_id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,
			from_value,to_value,cc_value,bcc_value,subject,snippet,body_text,body_html_sanitized,raw_source,
			seen,flagged,answered,draft,has_attachments,importance,dkim_status,spf_status,dmarc_status,phishing_score,
			remote_images_blocked,remote_images_allowed,date_header,internal_date,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"reply-1", collector.ID, "INBOX", 1, "collector-thread", "<reply-1@example.com>", "<root-1@example.com>", "<root-1@example.com>",
		"Lead <lead@example.com>", collector.Login, "", "", "Re: Hello", "I have a question", "I have a question", "", "raw",
		0, 0, 0, 0, 0, 0, "unknown", "unknown", "unknown", 0.0,
		1, 0, now, now, now, now,
	); err != nil {
		t.Fatalf("insert reply message: %v", err)
	}

	if err := svc.ApplyManualReplyOutcome(ctx, user, enrollment.ID, "question", 1); err != nil {
		t.Fatalf("ApplyManualReplyOutcome: %v", err)
	}

	updatedBinding, err := st.GetMailThreadBindingByEnrollment(ctx, enrollment.ID)
	if err != nil {
		t.Fatalf("load updated binding: %v", err)
	}
	if updatedBinding.ThreadID != "collector-thread" {
		t.Fatalf("expected collector thread id, got %q", updatedBinding.ThreadID)
	}
	if updatedBinding.ThreadSubject != "Re: Hello" {
		t.Fatalf("expected collector thread subject, got %q", updatedBinding.ThreadSubject)
	}
	if updatedBinding.LastReplyAt.IsZero() {
		t.Fatalf("expected last reply timestamp to be set")
	}

	updatedEnrollment, err := st.GetOutboundEnrollmentByID(ctx, user.ID, enrollment.ID)
	if err != nil {
		t.Fatalf("load updated enrollment: %v", err)
	}
	if updatedEnrollment.Status != "manual_only" {
		t.Fatalf("expected enrollment to require human review, got %q", updatedEnrollment.Status)
	}
	if updatedEnrollment.ReplyOutcome != "question" {
		t.Fatalf("expected reply outcome question, got %q", updatedEnrollment.ReplyOutcome)
	}
	if updatedEnrollment.LastReplyMessageID != "reply-1" {
		t.Fatalf("expected last reply message id reply-1, got %q", updatedEnrollment.LastReplyMessageID)
	}
}

func TestApplyOutboundPlaybookCreatesStepsAndPolicies(t *testing.T) {
	ctx := context.Background()
	svc, st, _ := newOutboundTestService(t)

	user, err := st.CreateUser(ctx, "playbook@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	campaign, err := st.CreateOutboundCampaign(ctx, models.OutboundCampaign{
		UserID:             user.ID,
		Name:               "Playbook Candidate",
		Status:             "draft",
		AudienceSourceKind: "manual",
		SenderPolicyKind:   "preferred_sender",
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}

	updated, steps, err := svc.ApplyOutboundPlaybook(ctx, user, campaign.ID, "thread_revival", true)
	if err != nil {
		t.Fatalf("ApplyOutboundPlaybook: %v", err)
	}
	if updated.CampaignMode != "existing_threads" {
		t.Fatalf("expected existing_threads mode, got %q", updated.CampaignMode)
	}
	if updated.SenderPolicyKind != "thread_owner" {
		t.Fatalf("expected thread_owner sender policy, got %q", updated.SenderPolicyKind)
	}
	if updated.PlaybookKey != "thread_revival" {
		t.Fatalf("expected playbook key thread_revival, got %q", updated.PlaybookKey)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 playbook steps, got %d", len(steps))
	}
	if steps[1].Kind != "manual_task" {
		t.Fatalf("expected step 2 to be manual_task, got %q", steps[1].Kind)
	}
}

func TestDispatchOutboundManualTaskStepEntersManualOnly(t *testing.T) {
	ctx := context.Background()
	svc, st, _ := newOutboundTestService(t)

	user, err := st.CreateUser(ctx, "manualtask@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	campaign, err := st.CreateOutboundCampaign(ctx, models.OutboundCampaign{
		UserID:             user.ID,
		Name:               "Manual Task Campaign",
		Status:             "running",
		AudienceSourceKind: "manual",
		SenderPolicyKind:   "preferred_sender",
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	if _, err := st.CreateOutboundCampaignStep(ctx, models.OutboundCampaignStep{
		CampaignID:          campaign.ID,
		Position:            1,
		Kind:                "manual_task",
		ThreadMode:          "same_thread",
		TaskPolicyJSON:      `{"title":"Review thread","instructions":"Respond manually"}`,
		WaitIntervalMinutes: 0,
	}); err != nil {
		t.Fatalf("create step: %v", err)
	}
	enrollment, err := st.UpsertOutboundEnrollment(ctx, models.OutboundEnrollment{
		CampaignID:      campaign.ID,
		RecipientEmail:  "lead@example.com",
		RecipientDomain: "example.com",
		Status:          "scheduled",
	})
	if err != nil {
		t.Fatalf("create enrollment: %v", err)
	}

	if err := svc.DispatchOutboundEnrollment(ctx, enrollment); err != nil {
		t.Fatalf("DispatchOutboundEnrollment: %v", err)
	}
	updated, err := st.GetOutboundEnrollmentByID(ctx, user.ID, enrollment.ID)
	if err != nil {
		t.Fatalf("get updated enrollment: %v", err)
	}
	if updated.Status != "manual_only" {
		t.Fatalf("expected manual_only, got %q", updated.Status)
	}
	if updated.CurrentStepPosition != 1 {
		t.Fatalf("expected current step position 1, got %d", updated.CurrentStepPosition)
	}
	if updated.PauseReason != "manual_task" {
		t.Fatalf("expected pause_reason manual_task, got %q", updated.PauseReason)
	}
}

func TestApplyManualReplyOutcomeBranchesIntoManualTask(t *testing.T) {
	ctx := context.Background()
	svc, st, _ := newOutboundTestService(t)

	user, err := st.CreateUser(ctx, "branch@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	seedAccount := createOutboundServiceAccount(t, st, user.ID, "acct-seed", "seed@example.com", MailProviderTypeGeneric)
	campaign, err := st.CreateOutboundCampaign(ctx, models.OutboundCampaign{
		UserID:             user.ID,
		Name:               "Branching Campaign",
		Status:             "running",
		AudienceSourceKind: "manual",
		SenderPolicyKind:   "preferred_sender",
	})
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	if _, err := st.CreateOutboundCampaignStep(ctx, models.OutboundCampaignStep{
		CampaignID:          campaign.ID,
		Position:            1,
		Kind:                "email",
		ThreadMode:          "same_thread",
		SubjectTemplate:     "Hi",
		BodyTemplate:        "Hello",
		BranchPolicyJSON:    `{"question":"manual_task:2"}`,
		WaitIntervalMinutes: 0,
	}); err != nil {
		t.Fatalf("create email step: %v", err)
	}
	if _, err := st.CreateOutboundCampaignStep(ctx, models.OutboundCampaignStep{
		CampaignID:          campaign.ID,
		Position:            2,
		Kind:                "manual_task",
		ThreadMode:          "same_thread",
		TaskPolicyJSON:      `{"title":"Review question"}`,
		WaitIntervalMinutes: 0,
	}); err != nil {
		t.Fatalf("create manual step: %v", err)
	}
	enrollment, err := st.UpsertOutboundEnrollment(ctx, models.OutboundEnrollment{
		CampaignID:          campaign.ID,
		RecipientEmail:      "lead@example.com",
		RecipientDomain:     "example.com",
		Status:              "waiting_reply",
		CurrentStepPosition: 1,
		LastSentMessageID:   "<sent-1@example.com>",
	})
	if err != nil {
		t.Fatalf("create enrollment: %v", err)
	}
	if _, err := st.UpsertMailThreadBinding(ctx, models.MailThreadBinding{
		AccountID:             seedAccount.ID,
		BindingType:           "campaign",
		CampaignID:            campaign.ID,
		EnrollmentID:          enrollment.ID,
		OwnerUserID:           user.ID,
		RecipientEmail:        enrollment.RecipientEmail,
		RecipientDomain:       enrollment.RecipientDomain,
		LastOutboundMessageID: "<sent-1@example.com>",
		LastReplyMessageID:    "reply-1",
		ThreadSubject:         "Re: Hi",
	}); err != nil {
		t.Fatalf("upsert binding: %v", err)
	}

	if err := svc.ApplyManualReplyOutcome(ctx, user, enrollment.ID, "question", 1); err != nil {
		t.Fatalf("ApplyManualReplyOutcome: %v", err)
	}
	updated, err := st.GetOutboundEnrollmentByID(ctx, user.ID, enrollment.ID)
	if err != nil {
		t.Fatalf("get updated enrollment: %v", err)
	}
	if updated.Status != "scheduled" {
		t.Fatalf("expected scheduled status for branched manual task, got %q", updated.Status)
	}
	if updated.CurrentStepPosition != 1 {
		t.Fatalf("expected current step position to remain 1 before manual task dispatch, got %d", updated.CurrentStepPosition)
	}

	if err := svc.DispatchOutboundEnrollment(ctx, updated); err != nil {
		t.Fatalf("DispatchOutboundEnrollment after branch: %v", err)
	}
	finalState, err := st.GetOutboundEnrollmentByID(ctx, user.ID, enrollment.ID)
	if err != nil {
		t.Fatalf("get final enrollment: %v", err)
	}
	if finalState.Status != "manual_only" {
		t.Fatalf("expected manual_only after manual task dispatch, got %q", finalState.Status)
	}
	if finalState.CurrentStepPosition != 2 {
		t.Fatalf("expected current step position 2, got %d", finalState.CurrentStepPosition)
	}
}
