package models

import "time"

type MailAccount struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	DisplayName  string    `json:"display_name"`
	Login        string    `json:"login"`
	SecretEnc    string    `json:"-"`
	IMAPHost     string    `json:"imap_host"`
	IMAPPort     int       `json:"imap_port"`
	IMAPTLS      bool      `json:"imap_tls"`
	IMAPStartTLS bool      `json:"imap_starttls"`
	SMTPHost     string    `json:"smtp_host"`
	SMTPPort     int       `json:"smtp_port"`
	SMTPTLS      bool      `json:"smtp_tls"`
	SMTPStartTLS bool      `json:"smtp_starttls"`
	IsDefault    bool      `json:"is_default"`
	Status       string    `json:"status"`
	LastSyncAt   time.Time `json:"last_sync_at,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type MailIdentity struct {
	ID            string    `json:"id"`
	AccountID     string    `json:"account_id"`
	DisplayName   string    `json:"display_name"`
	FromEmail     string    `json:"from_email"`
	ReplyTo       string    `json:"reply_to,omitempty"`
	SignatureText string    `json:"signature_text,omitempty"`
	SignatureHTML string    `json:"signature_html,omitempty"`
	IsDefault     bool      `json:"is_default"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type MailboxMapping struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	Role        string    `json:"role"`
	MailboxName string    `json:"mailbox_name"`
	Source      string    `json:"source"`
	Priority    int       `json:"priority"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ThreadSummary struct {
	ID             string    `json:"id"`
	AccountID      string    `json:"account_id"`
	Mailbox        string    `json:"mailbox"`
	SubjectNorm    string    `json:"subject_norm"`
	Participants   []string  `json:"participants"`
	MessageCount   int       `json:"message_count"`
	UnreadCount    int       `json:"unread_count"`
	HasAttachments bool      `json:"has_attachments"`
	HasFlagged     bool      `json:"has_flagged"`
	Importance     int       `json:"importance"`
	LatestMessage  string    `json:"latest_message_id"`
	LatestAt       time.Time `json:"latest_at"`
}

type IndexedMessage struct {
	ID                  string    `json:"id"`
	AccountID           string    `json:"account_id"`
	Source              string    `json:"source,omitempty"`
	Mailbox             string    `json:"mailbox"`
	UID                 uint32    `json:"uid"`
	ThreadID            string    `json:"thread_id"`
	MessageIDHeader     string    `json:"message_id_header,omitempty"`
	InReplyToHeader     string    `json:"in_reply_to_header,omitempty"`
	ReferencesHeader    string    `json:"references_header,omitempty"`
	FromValue           string    `json:"from"`
	ToValue             string    `json:"to"`
	CCValue             string    `json:"cc,omitempty"`
	BCCValue            string    `json:"bcc,omitempty"`
	Subject             string    `json:"subject"`
	Snippet             string    `json:"snippet"`
	BodyText            string    `json:"body"`
	BodyHTMLSanitized   string    `json:"body_html"`
	RawSource           string    `json:"raw_source"`
	Seen                bool      `json:"seen"`
	Flagged             bool      `json:"flagged"`
	Answered            bool      `json:"answered"`
	Draft               bool      `json:"draft"`
	HasAttachments      bool      `json:"has_attachments"`
	Importance          int       `json:"importance"`
	DKIMStatus          string    `json:"dkim_status"`
	SPFStatus           string    `json:"spf_status"`
	DMARCStatus         string    `json:"dmarc_status"`
	PhishingScore       float64   `json:"phishing_score"`
	RemoteImagesBlocked bool      `json:"remote_images_blocked"`
	RemoteImagesAllowed bool      `json:"remote_images_allowed"`
	DateHeader          time.Time `json:"date"`
	InternalDate        time.Time `json:"internal_date"`
}

type IndexedAttachment struct {
	ID          string    `json:"id"`
	MessageID   string    `json:"message_id"`
	AccountID   string    `json:"account_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	InlinePart  bool      `json:"inline_part"`
	CreatedAt   time.Time `json:"created_at"`
}

type ContactEmail struct {
	ID        string `json:"id"`
	ContactID string `json:"contact_id,omitempty"`
	Email     string `json:"email"`
	Label     string `json:"label,omitempty"`
	IsPrimary bool   `json:"is_primary"`
}

type Contact struct {
	ID                 string         `json:"id"`
	UserID             string         `json:"user_id,omitempty"`
	Name               string         `json:"name"`
	Nicknames          []string       `json:"nicknames,omitempty"`
	Emails             []ContactEmail `json:"emails,omitempty"`
	Notes              string         `json:"notes,omitempty"`
	GroupIDs           []string       `json:"group_ids,omitempty"`
	PreferredAccountID string         `json:"preferred_account_id,omitempty"`
	PreferredSenderID  string         `json:"preferred_sender_id,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type ContactGroup struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id,omitempty"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	MemberCount      int       `json:"member_count"`
	MemberContactIDs []string  `json:"member_contact_ids,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type RecipientSuggestion struct {
	Kind               string   `json:"kind,omitempty"`
	ID                 string   `json:"id,omitempty"`
	Email              string   `json:"email"`
	Label              string   `json:"label"`
	Subtitle           string   `json:"subtitle,omitempty"`
	Emails             []string `json:"emails,omitempty"`
	ContactID          string   `json:"contact_id,omitempty"`
	GroupID            string   `json:"group_id,omitempty"`
	MemberCount        int      `json:"member_count,omitempty"`
	Source             string   `json:"source,omitempty"`
	PreferredAccountID string   `json:"preferred_account_id,omitempty"`
	PreferredSenderID  string   `json:"preferred_sender_id,omitempty"`
}

type IndexedMessageFilter struct {
	Query          string
	From           string
	To             string
	Subject        string
	DateFrom       time.Time
	DateTo         time.Time
	HasDateFrom    bool
	HasDateTo      bool
	Unread         bool
	Flagged        bool
	HasAttachments bool
	Waiting        bool
	AccountIDs     []string
}

type UserPreferences struct {
	UserID            string    `json:"user_id"`
	Theme             string    `json:"theme"`
	Density           string    `json:"density"`
	LayoutMode        string    `json:"layout_mode"`
	KeymapJSON        string    `json:"keymap_json"`
	RemoteImagePolicy string    `json:"remote_image_policy"`
	Timezone          string    `json:"timezone"`
	PageSize          int       `json:"page_size"`
	GroupingMode      string    `json:"grouping_mode"`
	DefaultSenderID   string    `json:"default_sender_id"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type SavedSearch struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	AccountID   string    `json:"account_id"`
	Name        string    `json:"name"`
	FiltersJSON string    `json:"filters_json"`
	Pinned      bool      `json:"pinned"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Draft struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	AccountID        string    `json:"account_id"`
	IdentityID       string    `json:"identity_id"`
	SenderProfileID  string    `json:"sender_profile_id"`
	ComposeMode      string    `json:"compose_mode"`
	ContextMessageID string    `json:"context_message_id"`
	ContextAccountID string    `json:"context_account_id"`
	FromMode         string    `json:"from_mode"`
	FromManual       string    `json:"from_manual"`
	ClientStateJSON  string    `json:"client_state_json"`
	ToValue          string    `json:"to"`
	CCValue          string    `json:"cc"`
	BCCValue         string    `json:"bcc"`
	Subject          string    `json:"subject"`
	BodyText         string    `json:"body_text"`
	BodyHTML         string    `json:"body_html"`
	AttachmentsJSON  string    `json:"attachments_json"`
	CryptoOptions    string    `json:"crypto_options_json"`
	SendMode         string    `json:"send_mode"`
	ScheduledFor     time.Time `json:"scheduled_for,omitempty"`
	Status           string    `json:"status"`
	LastSendError    string    `json:"last_send_error,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type SenderProfile struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Name             string `json:"name"`
	FromEmail        string `json:"from_email"`
	ReplyTo          string `json:"reply_to,omitempty"`
	SignatureText    string `json:"signature_text,omitempty"`
	SignatureHTML    string `json:"signature_html,omitempty"`
	AccountID        string `json:"account_id"`
	AccountLabel     string `json:"account_label"`
	IsDefault        bool   `json:"is_default"`
	IsPrimary        bool   `json:"is_primary"`
	CanDelete        bool   `json:"can_delete"`
	CanSchedule      bool   `json:"can_schedule"`
	Status           string `json:"status"`
	AccountIsDefault bool   `json:"account_is_default,omitempty"`
	IsAccountDefault bool   `json:"is_account_default,omitempty"`
}

type DraftVersion struct {
	ID           string    `json:"id"`
	DraftID      string    `json:"draft_id"`
	VersionNo    int       `json:"version_no"`
	SnapshotJSON string    `json:"snapshot_json"`
	CreatedAt    time.Time `json:"created_at"`
}

type DraftAttachment struct {
	ID          string    `json:"id"`
	DraftID     string    `json:"draft_id"`
	UserID      string    `json:"user_id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	InlinePart  bool      `json:"inline_part"`
	ContentID   string    `json:"content_id,omitempty"`
	SortOrder   int       `json:"sort_order"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Data        []byte    `json:"-"`
}

type SessionMailProfile struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	FromEmail     string    `json:"from_email"`
	DisplayName   string    `json:"display_name"`
	ReplyTo       string    `json:"reply_to"`
	SignatureText string    `json:"signature_text"`
	SignatureHTML string    `json:"signature_html"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type SieveScript struct {
	ID          string    `json:"id"`
	AccountID   string    `json:"account_id"`
	ScriptName  string    `json:"script_name"`
	ScriptBody  string    `json:"script_body"`
	ChecksumSHA string    `json:"checksum_sha256"`
	IsActive    bool      `json:"is_active"`
	Source      string    `json:"source"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type MFAStatus struct {
	HasTOTP        bool `json:"has_totp"`
	TOTPEnabled    bool `json:"totp_enabled"`
	WebAuthnCount  int  `json:"webauthn_credentials"`
	RecoveryCodes  int  `json:"recovery_codes"`
	RecoveryUnused int  `json:"recovery_unused"`
}

type MFATOTPRecord struct {
	UserID      string    `json:"user_id"`
	SecretEnc   string    `json:"-"`
	Issuer      string    `json:"issuer"`
	AccountName string    `json:"account_name"`
	Enabled     bool      `json:"enabled"`
	EnrolledAt  time.Time `json:"enrolled_at,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type MFAWebAuthnCredential struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	CredentialID   string    `json:"credential_id"`
	PublicKey      string    `json:"public_key"`
	SignCount      int64     `json:"sign_count"`
	TransportsJSON string    `json:"transports_json"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
	LastUsedAt     time.Time `json:"last_used_at,omitempty"`
}

type MFATrustedDevice struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	TokenHash   string    `json:"-"`
	UAHash      string    `json:"-"`
	IPHint      string    `json:"ip_hint"`
	DeviceLabel string    `json:"device_label"`
	CreatedAt   time.Time `json:"created_at"`
	LastUsedAt  time.Time `json:"last_used_at,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	RevokedAt   time.Time `json:"revoked_at,omitempty"`
}

type SessionMeta struct {
	SessionID     string    `json:"session_id"`
	UserID        string    `json:"user_id"`
	DeviceLabel   string    `json:"device_label"`
	UASummary     string    `json:"ua_summary"`
	IPHint        string    `json:"ip_hint"`
	AuthMethod    string    `json:"auth_method"`
	MFAVerifiedAt time.Time `json:"mfa_verified_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	LastSeenAt    time.Time `json:"last_seen_at"`
	ExpiresAt     time.Time `json:"expires_at"`
	IdleExpiresAt time.Time `json:"idle_expires_at"`
	RevokedAt     time.Time `json:"revoked_at,omitempty"`
	RevokedReason string    `json:"revoked_reason,omitempty"`
}

type QuotaCache struct {
	ID            string    `json:"id"`
	AccountID     string    `json:"account_id"`
	UsedBytes     int64     `json:"used_bytes"`
	TotalBytes    int64     `json:"total_bytes"`
	UsedMessages  int64     `json:"used_messages"`
	TotalMessages int64     `json:"total_messages"`
	RefreshedAt   time.Time `json:"refreshed_at"`
	LastError     string    `json:"last_error,omitempty"`
}

type MailHealthActionState struct {
	Kind      string    `json:"kind,omitempty"`
	Status    string    `json:"status,omitempty"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type MailAccountHealth struct {
	AccountID        string                 `json:"account_id"`
	AccountLabel     string                 `json:"account_label"`
	IsDefault        bool                   `json:"is_default"`
	Status           string                 `json:"status"`
	LastSyncAt       time.Time              `json:"last_sync_at,omitempty"`
	LastError        string                 `json:"last_error,omitempty"`
	QuotaAvailable   bool                   `json:"quota_available"`
	QuotaSupported   bool                   `json:"quota_supported"`
	UsedBytes        int64                  `json:"used_bytes"`
	TotalBytes       int64                  `json:"total_bytes"`
	UsedMessages     int64                  `json:"used_messages"`
	TotalMessages    int64                  `json:"total_messages"`
	QuotaRefreshedAt time.Time              `json:"quota_refreshed_at,omitempty"`
	QuotaLastError   string                 `json:"quota_last_error,omitempty"`
	ActionState      *MailHealthActionState `json:"action_state,omitempty"`
}

type MailAccountHealthSummary struct {
	TotalAccounts     int `json:"total_accounts"`
	HealthyAccounts   int `json:"healthy_accounts"`
	AttentionAccounts int `json:"attention_accounts"`
	ErrorAccounts     int `json:"error_accounts"`
}

type ScheduledSendQueueItem struct {
	ID          string    `json:"id"`
	DraftID     string    `json:"draft_id"`
	UserID      string    `json:"user_id"`
	AccountID   string    `json:"account_id"`
	DueAt       time.Time `json:"due_at"`
	State       string    `json:"state"`
	RetryCount  int       `json:"retry_count"`
	NextRetryAt time.Time `json:"next_retry_at,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CryptoKeyring struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	AccountID      string    `json:"account_id"`
	Kind           string    `json:"kind"`
	Fingerprint    string    `json:"fingerprint"`
	UserIDsJSON    string    `json:"user_ids_json"`
	PublicKey      string    `json:"public_key"`
	PrivateKeyEnc  string    `json:"-"`
	PassphraseHint string    `json:"passphrase_hint,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	TrustLevel     string    `json:"trust_level"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CryptoTrustPolicy struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	AccountID        string    `json:"account_id"`
	SenderPattern    string    `json:"sender_pattern"`
	DomainPattern    string    `json:"domain_pattern"`
	MinTrustLevel    string    `json:"min_trust_level"`
	RequireSigned    bool      `json:"require_signed"`
	RequireEncrypted bool      `json:"require_encrypted"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
