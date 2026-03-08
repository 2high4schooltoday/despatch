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
	cfg          config.Config
	gh           *githubClient
	now          func() time.Time
	runtimeProbe updaterRuntimeProbeFunc
}

func NewManager(cfg config.Config) *Manager {
	return &Manager{
		cfg:          cfg,
		gh:           newGitHubClient(cfg),
		now:          func() time.Time { return time.Now().UTC() },
		runtimeProbe: defaultUpdaterRuntimeProbe,
	}
}

func (m *Manager) Status(ctx context.Context, st *store.Store, forceCheck bool) (StatusResponse, error) {
	runtimeStatus := m.runtimeProbe(ctx, m.cfg)
	configured, configDiagnostic := m.configurationStatus(runtimeStatus)
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
	if recovered, err := m.recoverStaleQueuedApply(runtimeStatus, status.Apply); err != nil {
		return StatusResponse{}, err
	} else {
		status.Apply = recovered
	}
	if status.Apply.State == ApplyStateIdle && configured {
		pendingPaths, err := pendingRequestPaths(m.cfg)
		if err != nil {
			if isUpdaterPermissionError(err) {
				return status, nil
			}
			return StatusResponse{}, err
		}
		if len(pendingPaths) > 0 {
			status.Apply.State = ApplyStateQueued
		}
	}
	return status, nil
}

func (m *Manager) QueueApply(ctx context.Context, st *store.Store, requestedBy, targetVersion, requestID string) (ApplyRequest, error) {
	runtimeStatus := m.runtimeProbe(ctx, m.cfg)
	current, err := readApplyStatusTolerant(statusPath(m.cfg))
	if err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	if recovered, err := m.recoverStaleQueuedApply(runtimeStatus, current); err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	} else {
		current = recovered
	}
	configured, _ := m.configurationStatus(runtimeStatus)
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
	if current.State == ApplyStateQueued || current.State == ApplyStateInProgress {
		return ApplyRequest{}, ErrUpdateInProgress
	}
	pendingPaths, err := pendingRequestPaths(m.cfg)
	if err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	if len(pendingPaths) > 0 {
		return ApplyRequest{}, ErrUpdateInProgress
	}
	req := ApplyRequest{
		RequestID:     requestID,
		RequestedAt:   m.now(),
		RequestedBy:   strings.TrimSpace(requestedBy),
		TargetVersion: target,
	}
	if err := ensureUpdaterRequestStatusDirectories(m.cfg); err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	reqQueuePath := requestQueuePath(req, m.cfg)
	if err := writeJSONAtomic(reqQueuePath, req, 0o640, updaterDirModeForPath(m.cfg, requestDir(m.cfg), 0o750)); err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	if err := writeJSONAtomic(statusPath(m.cfg), ApplyStatus{
		State:         ApplyStateQueued,
		RequestID:     req.RequestID,
		RequestedAt:   req.RequestedAt,
		TargetVersion: req.TargetVersion,
	}, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750)); err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	return req, nil
}

func (m *Manager) configurationStatus(runtimeStatus updaterRuntimeStatus) (bool, *ConfigDiagnostic) {
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
	servicePath := updaterServiceUnitPath(m.cfg)
	if _, err := os.Stat(servicePath); err != nil {
		detail := fmt.Sprintf("required updater service unit is missing: %s", servicePath)
		if !os.IsNotExist(err) {
			detail = fmt.Sprintf("cannot verify updater service unit at %s: %v", servicePath, err)
		}
		return false, &ConfigDiagnostic{
			Reason:     "updater_service_missing",
			Detail:     detail,
			RepairHint: updaterUnitInstallRepairHint(m.cfg),
		}
	}
	if ok, diag := m.checkWritablePath(requestDir(m.cfg), "request"); !ok {
		return false, diag
	}
	if ok, diag := m.checkWritablePath(statusDir(m.cfg), "status"); !ok {
		return false, diag
	}
	if diag := runtimeStatus.ConfigDiagnostic(m.cfg); diag != nil {
		return false, diag
	}
	return true, nil
}

func (m *Manager) checkWritablePath(dirPath, stage string) (bool, *ConfigDiagnostic) {
	reasonPrefix := strings.TrimSpace(stage)
	if reasonPrefix == "" {
		reasonPrefix = "path"
	}
	pathState := describePathState(dirPath, 5)
	repairHint := m.updaterPermissionRepairHint()
	if err := ensureUpdaterWritableDirectory(m.cfg, dirPath); err != nil {
		return false, &ConfigDiagnostic{
			Reason:     fmt.Sprintf("%s_dir_unwritable", reasonPrefix),
			Detail:     fmt.Sprintf("cannot access updater %s directory %s: %v (path_state=%s)", reasonPrefix, dirPath, err, pathState),
			RepairHint: repairHint,
		}
	}
	probePath := filepath.Join(dirPath, fmt.Sprintf(".write-check-%d", time.Now().UnixNano()))
	if err := os.WriteFile(probePath, []byte("ok"), 0o600); err != nil {
		return false, &ConfigDiagnostic{
			Reason:     fmt.Sprintf("%s_probe_failed", reasonPrefix),
			Detail:     fmt.Sprintf("write probe failed for updater %s directory %s: %v (path_state=%s)", reasonPrefix, dirPath, err, pathState),
			RepairHint: repairHint,
		}
	}
	if err := os.Remove(probePath); err != nil {
		return false, &ConfigDiagnostic{
			Reason:     fmt.Sprintf("%s_probe_failed", reasonPrefix),
			Detail:     fmt.Sprintf("write probe cleanup failed for updater %s directory %s: %v (path_state=%s)", reasonPrefix, dirPath, err, pathState),
			RepairHint: repairHint,
		}
	}
	return true, nil
}

func (m *Manager) updaterPermissionRepairHint() string {
	updateDir := filepath.Clean(m.cfg.UpdateBaseDir)
	dataDir := filepath.Clean(filepath.Dir(updateDir))
	request := filepath.Clean(requestDir(m.cfg))
	status := filepath.Clean(statusDir(m.cfg))
	lock := filepath.Clean(lockDir(m.cfg))
	work := filepath.Clean(workDir(m.cfg))
	backups := filepath.Clean(backupsDir(m.cfg))
	return fmt.Sprintf(
		"run as root: install -d -o despatch -g despatch -m 0750 %s && install -d -o root -g despatch -m 0750 %s && install -d -o root -g despatch -m 0770 %s %s && install -d -o root -g root -m 0750 %s %s %s",
		shQuote(dataDir),
		shQuote(updateDir),
		shQuote(request),
		shQuote(status),
		shQuote(lock),
		shQuote(work),
		shQuote(backups),
	)
}

func describePathState(path string, depth int) string {
	if depth <= 0 {
		depth = 1
	}
	cur := filepath.Clean(path)
	parts := make([]string, 0, depth)
	for i := 0; i < depth; i++ {
		info, err := os.Stat(cur)
		if err != nil {
			parts = append(parts, fmt.Sprintf("%s(err=%v)", cur, err))
		} else {
			parts = append(parts, fmt.Sprintf("%s(mode=%#o)", cur, info.Mode().Perm()))
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return strings.Join(parts, " -> ")
}

func shQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
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

func (m *Manager) recoverStaleQueuedApply(runtimeStatus updaterRuntimeStatus, current ApplyStatus) (ApplyStatus, error) {
	if current.State != ApplyStateQueued {
		return current, nil
	}
	requestedAt := current.RequestedAt
	if requestedAt.IsZero() {
		return current, nil
	}
	if m.now().Before(requestedAt.Add(updateQueuePickupGrace)) {
		return current, nil
	}
	if err := removePendingRequestPaths(m.cfg); err != nil {
		return ApplyStatus{}, err
	}
	current.State = ApplyStateFailed
	current.FinishedAt = m.now()
	current.Error = runtimeStatus.StaleQueueError()
	if err := writeJSONAtomic(statusPath(m.cfg), current, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750)); err != nil {
		return ApplyStatus{}, err
	}
	_ = ensureDespatchReadable(statusPath(m.cfg))
	return current, nil
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
	if isUpdaterPermissionError(err) {
		// Treat unreadable status files as unknown/idle and allow queueing a new request.
		return ApplyStatus{State: ApplyStateIdle}, nil
	}
	return ApplyStatus{}, err
}
