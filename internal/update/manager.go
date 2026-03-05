package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"despatch/internal/config"
	"despatch/internal/store"
	"despatch/internal/version"
)

const (
	settingLastCheckAt     = "update_last_check_at"
	settingETag            = "update_etag"
	settingLatestTag       = "update_latest_tag"
	settingLatestPublished = "update_latest_published_at"
	settingLatestURL       = "update_latest_html_url"
	settingLastCheckError  = "update_last_check_error"
	settingLastSuccessVer  = "update_last_success_version"
	settingLastSuccessAt   = "update_last_success_at"
)

var targetVersionRx = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Manager struct {
	cfg config.Config
	gh  *githubClient
	now func() time.Time
}

func NewManager(cfg config.Config) *Manager {
	return &Manager{
		cfg: cfg,
		gh:  newGitHubClient(cfg),
		now: func() time.Time { return time.Now().UTC() },
	}
}

func (m *Manager) Status(ctx context.Context, st *store.Store, forceCheck bool) (StatusResponse, error) {
	configured, configDiagnostic := m.configurationStatus()
	status := StatusResponse{
		Enabled:          m.cfg.UpdateEnabled,
		Configured:       configured,
		Current:          version.Current(),
		Apply:            ApplyStatus{State: ApplyStateIdle},
		ConfigDiagnostic: configDiagnostic,
	}
	var checkErr error
	if m.cfg.UpdateEnabled && (forceCheck || m.shouldRefresh(ctx, st)) {
		checkErr = m.refreshLatest(ctx, st)
		if forceCheck && checkErr != nil {
			return status, checkErr
		}
	}

	latestTag, _ := m.getSetting(ctx, st, settingLatestTag)
	if latestTag != "" {
		latestPublishedRaw, _ := m.getSetting(ctx, st, settingLatestPublished)
		latestURL, _ := m.getSetting(ctx, st, settingLatestURL)
		rel := &ReleaseInfo{TagName: latestTag, HTMLURL: latestURL}
		if latestPublishedRaw != "" {
			if parsed, err := time.Parse(time.RFC3339, latestPublishedRaw); err == nil {
				rel.PublishedAt = parsed
			}
		}
		status.Latest = rel
		status.UpdateAvailable = compareVersions(status.Current.Version, rel.TagName)
	}
	status.LastCheckedAt, _ = m.getSetting(ctx, st, settingLastCheckAt)
	status.LastCheckError, _ = m.getSetting(ctx, st, settingLastCheckError)
	if checkErr != nil && status.LastCheckError == "" {
		status.LastCheckError = checkErr.Error()
	}
	apply, err := readApplyStatusTolerant(statusPath(m.cfg))
	if err != nil {
		return StatusResponse{}, err
	}
	status.Apply = apply
	if status.Apply.State == ApplyStateIdle {
		if _, err := os.Stat(lockPath(m.cfg)); err == nil {
			status.Apply.State = ApplyStateInProgress
		}
	}
	return status, nil
}

func (m *Manager) QueueApply(ctx context.Context, st *store.Store, requestedBy, targetVersion, requestID string) (ApplyRequest, error) {
	configured, _ := m.configurationStatus()
	if !m.cfg.UpdateEnabled || !configured {
		return ApplyRequest{}, ErrUpdaterNotConfigured
	}
	target := strings.TrimSpace(targetVersion)
	if target != "" && !targetVersionRx.MatchString(target) {
		return ApplyRequest{}, ErrInvalidTargetVersion
	}
	if requestID = strings.TrimSpace(requestID); requestID == "" {
		requestID = uuid.NewString()
	}
	current, err := readApplyStatusTolerant(statusPath(m.cfg))
	if err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	if current.State == ApplyStateQueued || current.State == ApplyStateInProgress {
		return ApplyRequest{}, ErrUpdateInProgress
	}
	if _, err := os.Stat(lockPath(m.cfg)); err == nil {
		return ApplyRequest{}, ErrUpdateInProgress
	}
	req := ApplyRequest{
		RequestID:     requestID,
		RequestedAt:   m.now(),
		RequestedBy:   strings.TrimSpace(requestedBy),
		TargetVersion: target,
	}
	if err := ensureDirs([]string{requestDir(m.cfg), statusDir(m.cfg)}, 0o750); err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	if err := writeJSONAtomic(requestPath(m.cfg), req, 0o640); err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	if err := writeJSONAtomic(statusPath(m.cfg), ApplyStatus{
		State:         ApplyStateQueued,
		RequestID:     req.RequestID,
		RequestedAt:   req.RequestedAt,
		TargetVersion: req.TargetVersion,
	}, 0o640); err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	return req, nil
}

func (m *Manager) configurationStatus() (bool, *ConfigDiagnostic) {
	if !m.cfg.UpdateEnabled {
		return false, nil
	}
	unitPath := updaterPathUnitPath(m.cfg)
	if _, err := os.Stat(unitPath); err != nil {
		detail := fmt.Sprintf("required updater unit marker is missing: %s", unitPath)
		if !os.IsNotExist(err) {
			detail = fmt.Sprintf("cannot verify updater unit marker at %s: %v", unitPath, err)
		}
		return false, &ConfigDiagnostic{
			Reason: "updater_unit_missing",
			Detail: detail,
			RepairHint: fmt.Sprintf(
				"install despatch-updater.path and despatch-updater.service into %s, run systemctl daemon-reload, then enable despatch-updater.path",
				m.cfg.UpdateSystemdUnitDir,
			),
		}
	}
	if ok, diag := m.checkWritablePath(requestDir(m.cfg), "request"); !ok {
		return false, diag
	}
	if ok, diag := m.checkWritablePath(statusDir(m.cfg), "status"); !ok {
		return false, diag
	}
	return true, nil
}

func (m *Manager) checkWritablePath(dirPath, stage string) (bool, *ConfigDiagnostic) {
	reasonPrefix := strings.TrimSpace(stage)
	if reasonPrefix == "" {
		reasonPrefix = "path"
	}
	if err := os.MkdirAll(dirPath, 0o750); err != nil {
		return false, &ConfigDiagnostic{
			Reason: fmt.Sprintf("%s_dir_unwritable", reasonPrefix),
			Detail: fmt.Sprintf("cannot access updater %s directory %s: %v", reasonPrefix, dirPath, err),
			RepairHint: fmt.Sprintf(
				"ensure %s exists and is writable by the despatch service user (expected owner root:despatch with group write permissions)",
				dirPath,
			),
		}
	}
	probePath := filepath.Join(dirPath, fmt.Sprintf(".write-check-%d", time.Now().UnixNano()))
	if err := os.WriteFile(probePath, []byte("ok"), 0o600); err != nil {
		return false, &ConfigDiagnostic{
			Reason: fmt.Sprintf("%s_probe_failed", reasonPrefix),
			Detail: fmt.Sprintf("write probe failed for updater %s directory %s: %v", reasonPrefix, dirPath, err),
			RepairHint: fmt.Sprintf(
				"fix ownership/mode for %s and its parent directories so user 'despatch' can create and remove files",
				dirPath,
			),
		}
	}
	if err := os.Remove(probePath); err != nil {
		return false, &ConfigDiagnostic{
			Reason: fmt.Sprintf("%s_probe_failed", reasonPrefix),
			Detail: fmt.Sprintf("write probe cleanup failed for updater %s directory %s: %v", reasonPrefix, dirPath, err),
			RepairHint: fmt.Sprintf(
				"fix ownership/mode for %s and its parent directories so user 'despatch' can remove probe files",
				dirPath,
			),
		}
	}
	return true, nil
}

func (m *Manager) shouldRefresh(ctx context.Context, st *store.Store) bool {
	lastCheckRaw, _ := m.getSetting(ctx, st, settingLastCheckAt)
	if strings.TrimSpace(lastCheckRaw) == "" {
		return true
	}
	lastCheck, err := time.Parse(time.RFC3339, lastCheckRaw)
	if err != nil {
		return true
	}
	interval := time.Duration(m.cfg.UpdateCheckIntervalMin) * time.Minute
	return m.now().After(lastCheck.Add(interval))
}

func (m *Manager) refreshLatest(ctx context.Context, st *store.Store) error {
	etag, _ := m.getSetting(ctx, st, settingETag)
	latest, newETag, notModified, err := m.gh.latestRelease(ctx, etag)
	now := m.now().Format(time.RFC3339)
	_ = st.UpsertSetting(ctx, settingLastCheckAt, now)
	if err != nil {
		_ = st.UpsertSetting(ctx, settingLastCheckError, err.Error())
		return err
	}
	_ = st.UpsertSetting(ctx, settingLastCheckError, "")
	if newETag != "" {
		_ = st.UpsertSetting(ctx, settingETag, newETag)
	}
	if notModified {
		return nil
	}
	_ = st.UpsertSetting(ctx, settingLatestTag, strings.TrimSpace(latest.TagName))
	_ = st.UpsertSetting(ctx, settingLatestURL, strings.TrimSpace(latest.HTMLURL))
	if !latest.PublishedAt.IsZero() {
		_ = st.UpsertSetting(ctx, settingLatestPublished, latest.PublishedAt.UTC().Format(time.RFC3339))
	}
	return nil
}

func (m *Manager) MarkSuccess(ctx context.Context, st *store.Store, targetVersion string) {
	_ = st.UpsertSetting(ctx, settingLastSuccessVer, strings.TrimSpace(targetVersion))
	_ = st.UpsertSetting(ctx, settingLastSuccessAt, m.now().Format(time.RFC3339))
}

func (m *Manager) getSetting(ctx context.Context, st *store.Store, key string) (string, bool) {
	v, ok, err := st.GetSetting(ctx, key)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(v), ok
}

func compareVersions(current, latest string) bool {
	c := strings.TrimSpace(current)
	l := strings.TrimSpace(latest)
	if c == "" || l == "" {
		return false
	}
	if strings.EqualFold(c, l) {
		return false
	}
	trim := func(v string) string {
		return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	}
	return trim(c) != trim(l)
}

func ApplyErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrUpdaterNotConfigured):
		return "updater_not_configured"
	case errors.Is(err, ErrUpdateInProgress):
		return "update_in_progress"
	case errors.Is(err, ErrInvalidTargetVersion):
		return "invalid_target_version"
	default:
		return "update_request_failed"
	}
}

func readApplyStatusTolerant(path string) (ApplyStatus, error) {
	st, err := readApplyStatus(path)
	if err == nil {
		return st, nil
	}
	if os.IsPermission(err) {
		// Treat unreadable status files as unknown/idle and allow queueing a new request.
		return ApplyStatus{State: ApplyStateIdle}, nil
	}
	return ApplyStatus{}, err
}
