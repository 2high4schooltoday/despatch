package update

import (
	"errors"
	"time"

	"despatch/internal/version"
)

var (
	ErrUpdaterNotConfigured = errors.New("updater is not configured on this host")
	ErrUpdateInProgress     = errors.New("an update is already in progress")
	ErrInvalidTargetVersion = errors.New("invalid target version")
	ErrUpdateRequestFailed  = errors.New("failed to queue update request")
)

type ReleaseInfo struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
}

type ApplyRequest struct {
	RequestID     string    `json:"request_id"`
	RequestedAt   time.Time `json:"requested_at"`
	RequestedBy   string    `json:"requested_by"`
	Mode          string    `json:"mode,omitempty"`
	TargetVersion string    `json:"target_version,omitempty"`
}

type ApplyState string

const (
	ApplyStateIdle       ApplyState = "idle"
	ApplyStateQueued     ApplyState = "queued"
	ApplyStateInProgress ApplyState = "in_progress"
	ApplyStateCompleted  ApplyState = "completed"
	ApplyStateFailed     ApplyState = "failed"
	ApplyStateRolledBack ApplyState = "rolled_back"
)

const (
	ApplyModeApply   = "apply"
	ApplyModePrepare = "prepare"
)

type ApplyStatus struct {
	State         ApplyState `json:"state"`
	RequestID     string     `json:"request_id,omitempty"`
	RequestedAt   time.Time  `json:"requested_at,omitempty"`
	StartedAt     time.Time  `json:"started_at,omitempty"`
	FinishedAt    time.Time  `json:"finished_at,omitempty"`
	TargetVersion string     `json:"target_version,omitempty"`
	FromVersion   string     `json:"from_version,omitempty"`
	ToVersion     string     `json:"to_version,omitempty"`
	RolledBack    bool       `json:"rolled_back,omitempty"`
	Error         string     `json:"error,omitempty"`
}

type AutoUpdateState string

const (
	AutoUpdateStateIdle       AutoUpdateState = "idle"
	AutoUpdateStatePreparing  AutoUpdateState = "preparing"
	AutoUpdateStateDownloaded AutoUpdateState = "downloaded"
	AutoUpdateStateScheduled  AutoUpdateState = "scheduled"
	AutoUpdateStateApplying   AutoUpdateState = "applying"
	AutoUpdateStateFailed     AutoUpdateState = "failed"
)

type AutoUpdateStatus struct {
	Enabled       bool            `json:"enabled"`
	State         AutoUpdateState `json:"state"`
	TargetVersion string          `json:"target_version,omitempty"`
	DownloadedAt  time.Time       `json:"downloaded_at,omitempty"`
	ScheduledFor  time.Time       `json:"scheduled_for,omitempty"`
	Error         string          `json:"error,omitempty"`
}

type StatusResponse struct {
	Enabled          bool              `json:"enabled"`
	Configured       bool              `json:"configured"`
	Current          version.Info      `json:"current"`
	Latest           *ReleaseInfo      `json:"latest,omitempty"`
	LastCheckedAt    string            `json:"last_checked_at,omitempty"`
	LastCheckError   string            `json:"last_check_error,omitempty"`
	UpdateAvailable  bool              `json:"update_available"`
	Apply            ApplyStatus       `json:"apply"`
	AutoUpdate       AutoUpdateStatus  `json:"auto_update"`
	ConfigDiagnostic *ConfigDiagnostic `json:"config_diagnostic,omitempty"`
}

type ConfigDiagnostic struct {
	Reason     string `json:"reason,omitempty"`
	Detail     string `json:"detail,omitempty"`
	RepairHint string `json:"repair_hint,omitempty"`
}
