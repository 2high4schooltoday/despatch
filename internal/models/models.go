package models

import "time"

type UserStatus string

const (
	UserPending   UserStatus = "pending"
	UserActive    UserStatus = "active"
	UserSuspended UserStatus = "suspended"
	UserRejected  UserStatus = "rejected"
)

type User struct {
	ID                     string
	Email                  string
	RecoveryEmail          *string
	MailLogin              *string
	MFAPreference          string
	LegacyMFAPromptPending bool
	MFASetupSwitchUsed     bool
	MFABackupCompleted     bool
	PasswordHash           string
	Role                   string
	Status                 UserStatus
	ProvisionState         string
	ProvisionError         *string
	CreatedAt              time.Time
	ApprovedAt             *time.Time
	ApprovedBy             *string
	LastLoginAt            *time.Time
}

type Registration struct {
	ID            string
	Email         string
	MFAPreference string
	SourceIP      string
	UserAgentHash string
	CaptchaOK     bool
	Status        string
	CreatedAt     time.Time
	DecidedAt     *time.Time
	DecidedBy     *string
	Reason        *string
}

type Session struct {
	ID              string
	UserID          string
	TokenHash       string
	MailSecret      string
	IPHint          string
	UserAgentHash   string
	AuthMethod      string
	MFAVerifiedAt   *time.Time
	ActiveAccountID *string
	ExpiresAt       time.Time
	IdleExpiresAt   time.Time
	CreatedAt       time.Time
	LastSeenAt      time.Time
	RevokedAt       *time.Time
}

type PasswordResetToken struct {
	ID            string
	UserID        string
	TokenHash     string
	ReservationID *string
	ReservedAt    *time.Time
	ExpiresAt     time.Time
	UsedAt        *time.Time
	CreatedAt     time.Time
}

type AuditEntry struct {
	ID             string    `json:"id"`
	ActorUserID    string    `json:"actor_user_id"`
	ActorEmail     string    `json:"actor_email,omitempty"`
	Action         string    `json:"action"`
	Target         string    `json:"target"`
	TargetLabel    string    `json:"target_label,omitempty"`
	MetadataJSON   string    `json:"metadata_json"`
	SummaryCode    string    `json:"summary_code,omitempty"`
	SummaryText    string    `json:"summary_text,omitempty"`
	SummaryVersion int       `json:"summary_version,omitempty"`
	Severity       string    `json:"severity,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type RegistrationQuery struct {
	Status string
	Q      string
	Sort   string
	Order  string
	Limit  int
	Offset int
}

type UserQuery struct {
	Q              string
	Status         string
	Role           string
	ProvisionState string
	Sort           string
	Order          string
	Limit          int
	Offset         int
}

type AuditQuery struct {
	Q      string
	Action string
	Actor  string
	Target string
	From   time.Time
	To     time.Time
	Sort   string
	Order  string
	Limit  int
	Offset int
}
