package models

import "time"

type OutboundCampaign struct {
	ID                    string    `json:"id"`
	UserID                string    `json:"user_id,omitempty"`
	Name                  string    `json:"name"`
	Status                string    `json:"status"`
	GoalKind              string    `json:"goal_kind,omitempty"`
	PlaybookKey           string    `json:"playbook_key,omitempty"`
	CampaignMode          string    `json:"campaign_mode,omitempty"`
	AudienceSourceKind    string    `json:"audience_source_kind"`
	AudienceSourceRef     string    `json:"audience_source_ref,omitempty"`
	SenderPolicyKind      string    `json:"sender_policy_kind"`
	SenderPolicyRef       string    `json:"sender_policy_ref,omitempty"`
	ReplyPolicyJSON       string    `json:"reply_policy_json,omitempty"`
	SuppressionPolicyJSON string    `json:"suppression_policy_json,omitempty"`
	SchedulePolicyJSON    string    `json:"schedule_policy_json,omitempty"`
	CompliancePolicyJSON  string    `json:"compliance_policy_json,omitempty"`
	GovernancePolicyJSON  string    `json:"governance_policy_json,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
	LaunchedAt            time.Time `json:"launched_at,omitempty"`
	CompletedAt           time.Time `json:"completed_at,omitempty"`

	EnrollmentCount   int `json:"enrollment_count,omitempty"`
	SentCount         int `json:"sent_count,omitempty"`
	RepliedCount      int `json:"replied_count,omitempty"`
	PositiveCount     int `json:"positive_count,omitempty"`
	NegativeCount     int `json:"negative_count,omitempty"`
	PausedCount       int `json:"paused_count,omitempty"`
	BouncedCount      int `json:"bounced_count,omitempty"`
	UnsubscribedCount int `json:"unsubscribed_count,omitempty"`
	WaitingHumanCount int `json:"waiting_human_count,omitempty"`
}

type OutboundCampaignStep struct {
	ID                  string    `json:"id"`
	CampaignID          string    `json:"campaign_id"`
	Position            int       `json:"position"`
	Kind                string    `json:"kind"`
	ThreadMode          string    `json:"thread_mode"`
	SubjectTemplate     string    `json:"subject_template"`
	BodyTemplate        string    `json:"body_template"`
	WaitIntervalMinutes int       `json:"wait_interval_minutes"`
	SendWindowJSON      string    `json:"send_window_json,omitempty"`
	TaskPolicyJSON      string    `json:"task_policy_json,omitempty"`
	BranchPolicyJSON    string    `json:"branch_policy_json,omitempty"`
	StopIfReplied       bool      `json:"stop_if_replied"`
	StopIfClicked       bool      `json:"stop_if_clicked"`
	StopIfBooked        bool      `json:"stop_if_booked"`
	StopIfUnsubscribed  bool      `json:"stop_if_unsubscribed"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type OutboundEnrollment struct {
	ID                  string    `json:"id"`
	CampaignID          string    `json:"campaign_id"`
	ContactID           string    `json:"contact_id,omitempty"`
	RecipientEmail      string    `json:"recipient_email"`
	RecipientDomain     string    `json:"recipient_domain"`
	SenderAccountID     string    `json:"sender_account_id,omitempty"`
	SenderProfileID     string    `json:"sender_profile_id,omitempty"`
	ReplyFunnelID       string    `json:"reply_funnel_id,omitempty"`
	ThreadBindingID     string    `json:"thread_binding_id,omitempty"`
	Status              string    `json:"status"`
	CurrentStepPosition int       `json:"current_step_position"`
	LastSentMessageID   string    `json:"last_sent_message_id,omitempty"`
	LastSentAt          time.Time `json:"last_sent_at,omitempty"`
	NextActionAt        time.Time `json:"next_action_at,omitempty"`
	PauseReason         string    `json:"pause_reason,omitempty"`
	StopReason          string    `json:"stop_reason,omitempty"`
	ReplyOutcome        string    `json:"reply_outcome,omitempty"`
	ReplyConfidence     float64   `json:"reply_confidence,omitempty"`
	ManualOwnerUserID   string    `json:"manual_owner_user_id,omitempty"`
	SeedContextJSON     string    `json:"seed_context_json,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	ContactName        string    `json:"contact_name,omitempty"`
	CampaignName       string    `json:"campaign_name,omitempty"`
	SenderAccountLabel string    `json:"sender_account_label,omitempty"`
	SenderAccountLogin string    `json:"sender_account_login,omitempty"`
	SenderProfileName  string    `json:"sender_profile_name,omitempty"`
	ThreadSubject      string    `json:"thread_subject,omitempty"`
	LastReplyMessageID string    `json:"last_reply_message_id,omitempty"`
	LastReplyPreview   string    `json:"last_reply_preview,omitempty"`
	LastReplyBucket    string    `json:"last_reply_bucket,omitempty"`
	LastReplyAt        time.Time `json:"last_reply_at,omitempty"`
}

type OutboundEvent struct {
	ID               string    `json:"id"`
	CampaignID       string    `json:"campaign_id"`
	EnrollmentID     string    `json:"enrollment_id"`
	EventKind        string    `json:"event_kind"`
	EventPayloadJSON string    `json:"event_payload_json,omitempty"`
	ActorKind        string    `json:"actor_kind"`
	ActorRef         string    `json:"actor_ref,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type RecipientState struct {
	UserID            string    `json:"user_id,omitempty"`
	RecipientEmail    string    `json:"recipient_email"`
	PrimaryContactID  string    `json:"primary_contact_id,omitempty"`
	RecipientDomain   string    `json:"recipient_domain"`
	Status            string    `json:"status"`
	Scope             string    `json:"scope"`
	LastReplyAt       time.Time `json:"last_reply_at,omitempty"`
	LastReplyOutcome  string    `json:"last_reply_outcome,omitempty"`
	SuppressedUntil   time.Time `json:"suppressed_until,omitempty"`
	SuppressionReason string    `json:"suppression_reason,omitempty"`
	Notes             string    `json:"notes,omitempty"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type MailThreadBinding struct {
	ID                    string    `json:"id"`
	AccountID             string    `json:"account_id"`
	ThreadID              string    `json:"thread_id,omitempty"`
	BindingType           string    `json:"binding_type"`
	CampaignID            string    `json:"campaign_id,omitempty"`
	EnrollmentID          string    `json:"enrollment_id,omitempty"`
	FunnelID              string    `json:"funnel_id,omitempty"`
	ReplyAccountID        string    `json:"reply_account_id,omitempty"`
	ReplySenderProfileID  string    `json:"reply_sender_profile_id,omitempty"`
	CollectorAccountID    string    `json:"collector_account_id,omitempty"`
	OwnerUserID           string    `json:"owner_user_id,omitempty"`
	RecipientEmail        string    `json:"recipient_email,omitempty"`
	RecipientDomain       string    `json:"recipient_domain,omitempty"`
	RootOutboundMessageID string    `json:"root_outbound_message_id,omitempty"`
	LastOutboundMessageID string    `json:"last_outbound_message_id,omitempty"`
	LastReplyMessageID    string    `json:"last_reply_message_id,omitempty"`
	ThreadSubject         string    `json:"thread_subject,omitempty"`
	LastReplyAt           time.Time `json:"last_reply_at,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type OutboundSuppression struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id,omitempty"`
	ScopeKind  string    `json:"scope_kind"`
	ScopeValue string    `json:"scope_value"`
	CampaignID string    `json:"campaign_id,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	SourceKind string    `json:"source_kind"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type OutboundAudienceMember struct {
	ContactID            string `json:"contact_id,omitempty"`
	ContactName          string `json:"contact_name,omitempty"`
	RecipientEmail       string `json:"recipient_email"`
	RecipientDomain      string `json:"recipient_domain"`
	PreferredAccountID   string `json:"preferred_account_id,omitempty"`
	PreferredSenderID    string `json:"preferred_sender_id,omitempty"`
	ExistingEnrollmentID string `json:"existing_enrollment_id,omitempty"`
	SeedAccountID        string `json:"seed_account_id,omitempty"`
	SeedThreadID         string `json:"seed_thread_id,omitempty"`
	SeedMessageID        string `json:"seed_message_id,omitempty"`
	SeedThreadSubject    string `json:"seed_thread_subject,omitempty"`
	SeedMailbox          string `json:"seed_mailbox,omitempty"`
	ExistingThread       bool   `json:"existing_thread,omitempty"`
	Suppressed           bool   `json:"suppressed,omitempty"`
	SuppressionReason    string `json:"suppression_reason,omitempty"`
	ActiveElsewhere      bool   `json:"active_elsewhere,omitempty"`
}

type OutboundPreflightIssue struct {
	Code         string `json:"code"`
	Severity     string `json:"severity"`
	Recipient    string `json:"recipient,omitempty"`
	Domain       string `json:"domain,omitempty"`
	SenderRef    string `json:"sender_ref,omitempty"`
	Message      string `json:"message"`
	Blocking     bool   `json:"blocking"`
	CampaignID   string `json:"campaign_id,omitempty"`
	EnrollmentID string `json:"enrollment_id,omitempty"`
}

type ReplyOpsItem struct {
	ID                 string    `json:"id"`
	Bucket             string    `json:"bucket"`
	CampaignID         string    `json:"campaign_id,omitempty"`
	CampaignName       string    `json:"campaign_name,omitempty"`
	EnrollmentID       string    `json:"enrollment_id,omitempty"`
	ThreadBindingID    string    `json:"thread_binding_id,omitempty"`
	RecipientEmail     string    `json:"recipient_email"`
	RecipientDomain    string    `json:"recipient_domain"`
	SenderAccountID    string    `json:"sender_account_id,omitempty"`
	SenderAccountLabel string    `json:"sender_account_label,omitempty"`
	ReplyAccountID     string    `json:"reply_account_id,omitempty"`
	SenderProfileID    string    `json:"sender_profile_id,omitempty"`
	SenderProfileName  string    `json:"sender_profile_name,omitempty"`
	ReplyOutcome       string    `json:"reply_outcome"`
	ReplyConfidence    float64   `json:"reply_confidence"`
	MessageID          string    `json:"message_id,omitempty"`
	ThreadID           string    `json:"thread_id,omitempty"`
	ThreadSubject      string    `json:"thread_subject,omitempty"`
	Preview            string    `json:"preview,omitempty"`
	RecommendedAction  string    `json:"recommended_action,omitempty"`
	NeedsHuman         bool      `json:"needs_human"`
	CreatedAt          time.Time `json:"created_at"`
	LastReplyAt        time.Time `json:"last_reply_at,omitempty"`
	Status             string    `json:"status"`
}

type OutboundSenderDiagnostic struct {
	AccountID             string    `json:"account_id"`
	AccountLabel          string    `json:"account_label,omitempty"`
	AccountLogin          string    `json:"account_login,omitempty"`
	ProviderType          string    `json:"provider_type,omitempty"`
	Status                string    `json:"status"`
	LastSyncAt            time.Time `json:"last_sync_at,omitempty"`
	Sends24h              int       `json:"sends_24h"`
	Replies24h            int       `json:"replies_24h"`
	Bounces24h            int       `json:"bounces_24h"`
	WaitingReply          int       `json:"waiting_reply"`
	ActiveBindings        int       `json:"active_bindings"`
	RecommendedDailyCap   int       `json:"recommended_daily_cap,omitempty"`
	RecommendedHourlyCap  int       `json:"recommended_hourly_cap,omitempty"`
	RecommendedGapSeconds int       `json:"recommended_gap_seconds,omitempty"`
	CollectorAccountID    string    `json:"collector_account_id,omitempty"`
	CollectorAccountLabel string    `json:"collector_account_label,omitempty"`
	ReplyTopology         string    `json:"reply_topology,omitempty"`
}

type OutboundDomainDiagnostic struct {
	Domain            string    `json:"domain"`
	ActiveEnrollments int       `json:"active_enrollments"`
	PausedEnrollments int       `json:"paused_enrollments"`
	RepliedCount      int       `json:"replied_count"`
	Suppressed        bool      `json:"suppressed"`
	SuppressionReason string    `json:"suppression_reason,omitempty"`
	LastReplyAt       time.Time `json:"last_reply_at,omitempty"`
}

type OutboundPlaybook struct {
	Key                string                 `json:"key"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description,omitempty"`
	GoalKind           string                 `json:"goal_kind,omitempty"`
	CampaignMode       string                 `json:"campaign_mode,omitempty"`
	AudienceSourceKind string                 `json:"audience_source_kind,omitempty"`
	SenderPolicyKind   string                 `json:"sender_policy_kind,omitempty"`
	ReplyPolicy        map[string]any         `json:"reply_policy,omitempty"`
	SuppressionPolicy  map[string]any         `json:"suppression_policy,omitempty"`
	SchedulePolicy     map[string]any         `json:"schedule_policy,omitempty"`
	CompliancePolicy   map[string]any         `json:"compliance_policy,omitempty"`
	GovernancePolicy   map[string]any         `json:"governance_policy,omitempty"`
	Steps              []OutboundPlaybookStep `json:"steps,omitempty"`
}

type OutboundPlaybookStep struct {
	Position            int            `json:"position"`
	Kind                string         `json:"kind,omitempty"`
	ThreadMode          string         `json:"thread_mode,omitempty"`
	SubjectTemplate     string         `json:"subject_template,omitempty"`
	BodyTemplate        string         `json:"body_template,omitempty"`
	WaitIntervalMinutes int            `json:"wait_interval_minutes,omitempty"`
	TaskPolicy          map[string]any `json:"task_policy,omitempty"`
	BranchPolicy        map[string]any `json:"branch_policy,omitempty"`
}
