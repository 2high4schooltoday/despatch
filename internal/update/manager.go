package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
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
	settingAutoUpdateOn    = "update_auto_enabled"
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
	if err := m.recoverStaleAutoPrepare(runtimeStatus); err != nil {
		return StatusResponse{}, err
	}
	autoEnabled, _ := m.autoUpdateEnabled(ctx, st)
	autoRecord, err := readAutoUpdateState(autoStatusPath(m.cfg))
	if err != nil && !isUpdaterPermissionError(err) {
		return StatusResponse{}, err
	}
	status := StatusResponse{
		Enabled:          m.cfg.UpdateEnabled,
		Configured:       configured,
		Current:          version.Current(),
		Apply:            ApplyStatus{State: ApplyStateIdle},
		AutoUpdate:       autoStatusFromRecord(autoRecord, autoEnabled),
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
	status.Apply = m.staleQueuedApplyStatus(runtimeStatus, status.Apply)
	if status.Apply.State == ApplyStateIdle && configured {
		pending, err := pendingRequests(m.cfg)
		if err != nil {
			if isUpdaterPermissionError(err) {
				return status, nil
			}
			return StatusResponse{}, err
		}
		for _, req := range pending {
			if normalizeApplyMode(req.Request.Mode) == ApplyModeApply {
				status.Apply.State = ApplyStateQueued
				break
			}
		}
	}
	status.AutoUpdate = m.presentAutoUpdateStatus(status, autoRecord, autoEnabled)
	return status, nil
}

func (m *Manager) QueueApply(ctx context.Context, st *store.Store, requestedBy, targetVersion, requestID string) (ApplyRequest, error) {
	return m.queueRequest(ctx, st, requestedBy, targetVersion, requestID, ApplyModeApply)
}

func (m *Manager) QueuePrepare(ctx context.Context, st *store.Store, requestedBy, targetVersion, requestID string) (ApplyRequest, error) {
	return m.queueRequest(ctx, st, requestedBy, targetVersion, requestID, ApplyModePrepare)
}

func (m *Manager) queueRequest(ctx context.Context, st *store.Store, requestedBy, targetVersion, requestID, mode string) (ApplyRequest, error) {
	runtimeStatus := m.runtimeProbe(ctx, m.cfg)
	if err := m.recoverStaleAutoPrepare(runtimeStatus); err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
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
	mode = normalizeApplyMode(mode)
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
		Mode:          mode,
		TargetVersion: target,
	}
	if err := ensureUpdaterRequestStatusDirectories(m.cfg); err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	reqQueuePath := requestQueuePath(req, m.cfg)
	if err := writeJSONAtomic(reqQueuePath, req, 0o640, updaterDirModeForPath(m.cfg, requestDir(m.cfg), 0o750)); err != nil {
		return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
	}
	if mode == ApplyModeApply {
		if err := writeJSONAtomic(statusPath(m.cfg), ApplyStatus{
			State:         ApplyStateQueued,
			RequestID:     req.RequestID,
			RequestedAt:   req.RequestedAt,
			TargetVersion: req.TargetVersion,
		}, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750)); err != nil {
			return ApplyRequest{}, fmt.Errorf("%w: %v", ErrUpdateRequestFailed, err)
		}
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
	info, err := os.Stat(dirPath)
	if err != nil {
		return false, &ConfigDiagnostic{
			Reason:     fmt.Sprintf("%s_dir_unwritable", reasonPrefix),
			Detail:     fmt.Sprintf("cannot access updater %s directory %s: %v (path_state=%s)", reasonPrefix, dirPath, err, pathState),
			RepairHint: repairHint,
		}
	}
	if !info.IsDir() {
		return false, &ConfigDiagnostic{
			Reason:     fmt.Sprintf("%s_dir_unwritable", reasonPrefix),
			Detail:     fmt.Sprintf("updater %s path %s is not a directory (path_state=%s)", reasonPrefix, dirPath, pathState),
			RepairHint: repairHint,
		}
	}
	if !dirWritableByCurrentProcess(info) {
		return false, &ConfigDiagnostic{
			Reason:     fmt.Sprintf("%s_dir_unwritable", reasonPrefix),
			Detail:     fmt.Sprintf("updater %s directory %s is not writable/searchable by the current service user (path_state=%s)", reasonPrefix, dirPath, pathState),
			RepairHint: repairHint,
		}
	}
	return true, nil
}

func dirWritableByCurrentProcess(info os.FileInfo) bool {
	if !info.IsDir() {
		return false
	}
	if os.Geteuid() == 0 {
		return true
	}
	perm := info.Mode().Perm()
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return perm&0o003 == 0o003
	}
	if uint32(os.Geteuid()) == stat.Uid {
		return perm&0o300 == 0o300
	}
	if processInGroup(stat.Gid) {
		return perm&0o030 == 0o030
	}
	return perm&0o003 == 0o003
}

func processInGroup(gid uint32) bool {
	if uint32(os.Getegid()) == gid {
		return true
	}
	groups, err := os.Getgroups()
	if err != nil {
		return false
	}
	for _, groupID := range groups {
		if uint32(groupID) == gid {
			return true
		}
	}
	return false
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

func normalizeApplyMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ApplyModeApply:
		return ApplyModeApply
	case ApplyModePrepare:
		return ApplyModePrepare
	default:
		return ApplyModeApply
	}
}

func autoStatusFromRecord(rec autoUpdateStateRecord, enabled bool) AutoUpdateStatus {
	state := rec.State
	if state == "" {
		state = AutoUpdateStateIdle
	}
	return AutoUpdateStatus{
		Enabled:       enabled,
		State:         state,
		TargetVersion: strings.TrimSpace(rec.TargetVersion),
		DownloadedAt:  rec.DownloadedAt,
		ScheduledFor:  rec.ScheduledFor,
		Error:         strings.TrimSpace(rec.Error),
	}
}

func (m *Manager) presentAutoUpdateStatus(status StatusResponse, rec autoUpdateStateRecord, enabled bool) AutoUpdateStatus {
	auto := autoStatusFromRecord(rec, enabled)
	if auto.State == "" {
		auto.State = AutoUpdateStateIdle
	}
	if rec.State == AutoUpdateStateScheduled || rec.State == AutoUpdateStateDownloaded || rec.State == AutoUpdateStatePreparing {
		if status.Apply.State == ApplyStateQueued || status.Apply.State == ApplyStateInProgress {
			target := strings.TrimSpace(status.Apply.TargetVersion)
			if target == "" && status.Latest != nil {
				target = strings.TrimSpace(status.Latest.TagName)
			}
			if target != "" && target == strings.TrimSpace(rec.TargetVersion) {
				auto.State = AutoUpdateStateApplying
			}
		}
	}
	if auto.State == AutoUpdateStateIdle && enabled && status.UpdateAvailable && status.Latest != nil {
		auto.TargetVersion = strings.TrimSpace(status.Latest.TagName)
	}
	return auto
}

func (m *Manager) autoUpdateEnabled(ctx context.Context, st *store.Store) (bool, error) {
	raw, ok, err := st.GetSetting(ctx, settingAutoUpdateOn)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "0", "false", "off", "disabled", "no":
		return false, nil
	default:
		return true, nil
	}
}

func (m *Manager) AutomaticEnabled(ctx context.Context, st *store.Store) (bool, error) {
	return m.autoUpdateEnabled(ctx, st)
}

func (m *Manager) PersistAutomaticPreference(ctx context.Context, st *store.Store, actorUserID string, enabled bool) error {
	if err := st.UpsertSetting(ctx, settingAutoUpdateOn, boolToSetting(enabled)); err != nil {
		return err
	}
	_ = st.InsertAudit(ctx, strings.TrimSpace(actorUserID), "update_auto", "automatic_updates", fmt.Sprintf(`{"enabled":%t}`, enabled))
	return nil
}

func (m *Manager) SetAutomaticEnabled(ctx context.Context, st *store.Store, actorUserID string, enabled bool) (AutoUpdateStatus, error) {
	if err := m.PersistAutomaticPreference(ctx, st, actorUserID, enabled); err != nil {
		return AutoUpdateStatus{}, err
	}
	rec, err := readAutoUpdateState(autoStatusPath(m.cfg))
	if err != nil && !isUpdaterPermissionError(err) {
		return AutoUpdateStatus{}, err
	}
	if !enabled && rec.State == AutoUpdateStateScheduled {
		rec.State = AutoUpdateStateDownloaded
		rec.ScheduledFor = time.Time{}
	}
	if err := writeJSONAtomic(autoStatusPath(m.cfg), rec, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750)); err != nil && !isUpdaterPermissionError(err) {
		return AutoUpdateStatus{}, err
	}
	return autoStatusFromRecord(rec, enabled), nil
}

func (m *Manager) CancelScheduledUpdate(ctx context.Context, st *store.Store, actorUserID string) (AutoUpdateStatus, error) {
	enabled, err := m.autoUpdateEnabled(ctx, st)
	if err != nil {
		return AutoUpdateStatus{}, err
	}
	rec, err := readAutoUpdateState(autoStatusPath(m.cfg))
	if err != nil {
		return AutoUpdateStatus{}, err
	}
	rec.DeferredVersion = strings.TrimSpace(rec.TargetVersion)
	rec.State = AutoUpdateStateDownloaded
	rec.ScheduledFor = time.Time{}
	rec.Error = ""
	if err := writeJSONAtomic(autoStatusPath(m.cfg), rec, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750)); err != nil {
		return AutoUpdateStatus{}, err
	}
	_ = st.InsertAudit(ctx, strings.TrimSpace(actorUserID), "update_auto", "cancel_scheduled_update", fmt.Sprintf(`{"target_version":%q}`, rec.TargetVersion))
	return autoStatusFromRecord(rec, enabled), nil
}

func boolToSetting(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func nextNightlyWindow(now time.Time) time.Time {
	local := now.In(time.Local)
	scheduled := time.Date(local.Year(), local.Month(), local.Day(), 2, 0, 0, 0, local.Location())
	if !local.Before(scheduled) {
		scheduled = scheduled.Add(24 * time.Hour)
	}
	return scheduled
}

func (m *Manager) AutomaticTick(ctx context.Context, st *store.Store) error {
	if !m.cfg.UpdateEnabled {
		return nil
	}
	status, err := m.Status(ctx, st, false)
	if err != nil {
		return err
	}
	if !status.Configured {
		return nil
	}
	enabled := status.AutoUpdate.Enabled
	rec, err := readAutoUpdateState(autoStatusPath(m.cfg))
	if err != nil && !isUpdaterPermissionError(err) {
		return err
	}
	if rec.State == "" {
		rec.State = AutoUpdateStateIdle
	}
	latestTag := ""
	if status.Latest != nil {
		latestTag = strings.TrimSpace(status.Latest.TagName)
	}
	if latestTag == "" || !status.UpdateAvailable {
		if rec.State != AutoUpdateStateIdle || rec.TargetVersion != "" || rec.Error != "" || rec.DeferredVersion != "" {
			rec = autoUpdateStateRecord{State: AutoUpdateStateIdle}
			_ = writeJSONAtomic(autoStatusPath(m.cfg), rec, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750))
		}
		return nil
	}
	if !enabled {
		if rec.State == AutoUpdateStateScheduled {
			rec.State = AutoUpdateStateDownloaded
			rec.ScheduledFor = time.Time{}
			rec.Error = ""
			_ = writeJSONAtomic(autoStatusPath(m.cfg), rec, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750))
		}
		return nil
	}
	if rec.TargetVersion != "" && rec.TargetVersion != latestTag {
		rec = autoUpdateStateRecord{State: AutoUpdateStateIdle}
		_ = writeJSONAtomic(autoStatusPath(m.cfg), rec, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750))
	}
	if rec.DeferredVersion == latestTag {
		return nil
	}
	switch rec.State {
	case AutoUpdateStatePreparing, AutoUpdateStateScheduled, AutoUpdateStateApplying:
		if rec.State == AutoUpdateStateScheduled && !rec.ScheduledFor.IsZero() && !m.now().Before(rec.ScheduledFor) && status.Apply.State == ApplyStateIdle {
			_, err := m.queueRequest(ctx, st, "system:auto-update", latestTag, "auto-apply-"+sanitizePathToken(latestTag), ApplyModeApply)
			if err == nil || errors.Is(err, ErrUpdateInProgress) {
				return nil
			}
			rec.State = AutoUpdateStateFailed
			rec.Error = err.Error()
			_ = writeJSONAtomic(autoStatusPath(m.cfg), rec, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750))
			return nil
		}
		return nil
	case AutoUpdateStateDownloaded:
		if rec.DeferredVersion == latestTag {
			return nil
		}
		rec.State = AutoUpdateStateScheduled
		rec.ScheduledFor = nextNightlyWindow(m.now())
		rec.Error = ""
		_ = writeJSONAtomic(autoStatusPath(m.cfg), rec, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750))
		return nil
	case AutoUpdateStateFailed:
		if strings.TrimSpace(rec.TargetVersion) == latestTag {
			return nil
		}
	}
	rec = autoUpdateStateRecord{
		State:         AutoUpdateStatePreparing,
		TargetVersion: latestTag,
		Error:         "",
	}
	if err := writeJSONAtomic(autoStatusPath(m.cfg), rec, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750)); err != nil {
		return err
	}
	_, err = m.queueRequest(ctx, st, "system:auto-update", latestTag, "auto-prepare-"+sanitizePathToken(latestTag), ApplyModePrepare)
	if err != nil && !errors.Is(err, ErrUpdateInProgress) {
		rec.State = AutoUpdateStateFailed
		rec.Error = err.Error()
		_ = writeJSONAtomic(autoStatusPath(m.cfg), rec, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750))
	}
	return nil
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
	next := m.staleQueuedApplyStatus(runtimeStatus, current)
	if next.State == current.State && next.Error == current.Error && next.FinishedAt.Equal(current.FinishedAt) {
		return current, nil
	}
	if err := removePendingRequestPaths(m.cfg); err != nil {
		return ApplyStatus{}, err
	}
	if err := writeJSONAtomic(statusPath(m.cfg), next, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750)); err != nil {
		return ApplyStatus{}, err
	}
	_ = ensureDespatchReadable(statusPath(m.cfg))
	return next, nil
}

func (m *Manager) staleQueuedApplyStatus(runtimeStatus updaterRuntimeStatus, current ApplyStatus) ApplyStatus {
	if current.State != ApplyStateQueued {
		return current
	}
	requestedAt := current.RequestedAt
	if requestedAt.IsZero() {
		return current
	}
	if m.now().Before(requestedAt.Add(updateQueuePickupGrace)) {
		return current
	}
	current.State = ApplyStateFailed
	current.FinishedAt = m.now()
	current.Error = runtimeStatus.StaleQueueError()
	return current
}

func (m *Manager) recoverStaleAutoPrepare(runtimeStatus updaterRuntimeStatus) error {
	rec, err := readAutoUpdateState(autoStatusPath(m.cfg))
	if err != nil {
		if isUpdaterPermissionError(err) {
			return nil
		}
		return err
	}
	if rec.State != AutoUpdateStatePreparing {
		return nil
	}
	pending, err := pendingRequests(m.cfg)
	if err != nil {
		if isUpdaterPermissionError(err) {
			return nil
		}
		return err
	}
	stalePaths := make([]string, 0, 1)
	for _, item := range pending {
		if normalizeApplyMode(item.Request.Mode) != ApplyModePrepare {
			continue
		}
		if rec.TargetVersion != "" {
			reqTarget := strings.TrimSpace(item.Request.TargetVersion)
			if reqTarget != "" && reqTarget != rec.TargetVersion {
				continue
			}
		}
		requestedAt := item.Request.RequestedAt
		if requestedAt.IsZero() {
			if info, statErr := os.Stat(item.Path); statErr == nil {
				requestedAt = info.ModTime().UTC()
			}
		}
		if requestedAt.IsZero() || m.now().Before(requestedAt.Add(updateQueuePickupGrace)) {
			return nil
		}
		stalePaths = append(stalePaths, item.Path)
	}
	if len(stalePaths) == 0 {
		return nil
	}
	for _, path := range stalePaths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	rec.State = AutoUpdateStateFailed
	rec.DownloadedAt = time.Time{}
	rec.ScheduledFor = time.Time{}
	rec.Error = runtimeStatus.StaleQueueError()
	return writeAutoUpdateState(m.cfg, rec)
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
