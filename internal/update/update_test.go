package update

import (
	"context"
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/store"
)

func newUpdateTestStore(t *testing.T) *store.Store {
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
	return store.New(sqdb)
}

func dirPerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}

func writeUpdaterUnitFiles(t *testing.T, unitDir string) {
	t.Helper()
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	workerPath := filepath.Join(unitDir, "fake-update-worker")
	if err := os.WriteFile(workerPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake updater worker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "despatch-updater.path"), []byte("[Path]\nUnit=despatch-updater.service\n"), 0o644); err != nil {
		t.Fatalf("write updater path unit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "despatch-updater.service"), []byte("[Service]\nExecStart="+workerPath+"\n"), 0o644); err != nil {
		t.Fatalf("write updater service unit: %v", err)
	}
}

func makeLockDirUnreadable(t *testing.T, cfg config.Config) {
	t.Helper()
	if err := os.MkdirAll(lockDir(cfg), 0o750); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	if err := os.Chmod(lockDir(cfg), 0o000); err != nil {
		t.Fatalf("chmod lock dir unreadable: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(lockDir(cfg), 0o750)
	})
}

func installFakeSystemctl(t *testing.T, pathLoad, pathActive, serviceLoad, serviceActive string) {
	installFakeSystemctlWithDetails(t, pathLoad, pathActive, "", "", serviceLoad, serviceActive, "", "")
}

func installFakeSystemctlWithDetails(t *testing.T, pathLoad, pathActive, pathSubState, pathResult, serviceLoad, serviceActive, serviceSubState, serviceResult string) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "systemctl")
	body := fmt.Sprintf(`#!/bin/sh
prop=""
unit=""
for arg in "$@"; do
  case "$arg" in
    --property=*) prop="${arg#--property=}" ;;
    show|--value) ;;
    *) unit="$arg" ;;
  esac
done
case "${unit}:${prop}" in
  despatch-updater.path:LoadState) printf '%%s\n' %q ;;
  despatch-updater.path:ActiveState) printf '%%s\n' %q ;;
  despatch-updater.path:SubState) printf '%%s\n' %q ;;
  despatch-updater.path:Result) printf '%%s\n' %q ;;
  despatch-updater.service:LoadState) printf '%%s\n' %q ;;
  despatch-updater.service:ActiveState) printf '%%s\n' %q ;;
  despatch-updater.service:SubState) printf '%%s\n' %q ;;
  despatch-updater.service:Result) printf '%%s\n' %q ;;
  *) printf '%%s\n' "" ;;
esac
`, pathLoad, pathActive, pathSubState, pathResult, serviceLoad, serviceActive, serviceSubState, serviceResult)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake systemctl: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCompareVersions(t *testing.T) {
	t.Parallel()
	if compareVersions("v1.2.3", "v1.2.3") {
		t.Fatalf("expected same version to not be an update")
	}
	if !compareVersions("v1.2.3", "v1.2.4") {
		t.Fatalf("expected newer version to be detected")
	}
	if compareVersions("", "v1.2.4") {
		t.Fatalf("expected empty current version to not claim update")
	}
}

func TestQueueApplyValidationAndInProgress(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	mgr := NewManager(cfg)

	if _, err := mgr.QueueApply(context.Background(), st, "admin@example.com", "bad version!", ""); err == nil {
		t.Fatalf("expected invalid target version error")
	}
	req, err := mgr.QueueApply(context.Background(), st, "admin@example.com", "v1.2.3", "req-1")
	if err != nil {
		t.Fatalf("queue apply: %v", err)
	}
	if req.RequestID != "req-1" {
		t.Fatalf("unexpected request id: %q", req.RequestID)
	}
	if got := dirPerm(t, cfg.UpdateBaseDir); got != 0o750 {
		t.Fatalf("expected update base mode 0750, got %#o", got)
	}
	if got := dirPerm(t, requestDir(cfg)); got != 0o770 {
		t.Fatalf("expected request dir mode 0770, got %#o", got)
	}
	if got := dirPerm(t, statusDir(cfg)); got != 0o770 {
		t.Fatalf("expected status dir mode 0770, got %#o", got)
	}
	if _, err := mgr.QueueApply(context.Background(), st, "admin@example.com", "v1.2.4", "req-2"); err == nil {
		t.Fatalf("expected in-progress protection on second request")
	}
}

func TestAutomaticUpdatesDefaultOnAndCanBeDisabled(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	mgr := NewManager(cfg)

	enabled, err := mgr.autoUpdateEnabled(context.Background(), st)
	if err != nil {
		t.Fatalf("autoUpdateEnabled default: %v", err)
	}
	if !enabled {
		t.Fatalf("expected automatic updates to default on")
	}

	auto, err := mgr.SetAutomaticEnabled(context.Background(), st, "admin@example.com", false)
	if err != nil {
		t.Fatalf("disable automatic updates: %v", err)
	}
	if auto.Enabled {
		t.Fatalf("expected automatic updates disabled")
	}
}

func TestAutomaticTickQueuesPrepareForLatestRelease(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	mgr := NewManager(cfg)
	ctx := context.Background()
	_ = st.UpsertSetting(ctx, settingLastCheckAt, time.Now().UTC().Format(time.RFC3339))
	_ = st.UpsertSetting(ctx, settingLatestTag, "v9.9.9")
	_ = st.UpsertSetting(ctx, settingLatestPublished, time.Now().UTC().Format(time.RFC3339))

	if err := mgr.AutomaticTick(ctx, st); err != nil {
		t.Fatalf("automatic tick: %v", err)
	}
	rec, err := readAutoUpdateState(autoStatusPath(cfg))
	if err != nil {
		t.Fatalf("read auto state: %v", err)
	}
	if rec.State != AutoUpdateStatePreparing {
		t.Fatalf("expected preparing state, got %q", rec.State)
	}
	pending, err := pendingRequests(cfg)
	if err != nil {
		t.Fatalf("pending requests: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected one pending request, got %d", len(pending))
	}
	if pending[0].Request.Mode != ApplyModePrepare {
		t.Fatalf("expected prepare request, got %q", pending[0].Request.Mode)
	}
}

func TestAutomaticTickNoopsWhenUpdaterRuntimeIsInactive(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "inactive", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	mgr := NewManager(cfg)
	ctx := context.Background()
	_ = st.UpsertSetting(ctx, settingLastCheckAt, time.Now().UTC().Format(time.RFC3339))
	_ = st.UpsertSetting(ctx, settingLatestTag, "v9.9.9")
	_ = st.UpsertSetting(ctx, settingLatestPublished, time.Now().UTC().Format(time.RFC3339))

	if err := mgr.AutomaticTick(ctx, st); err != nil {
		t.Fatalf("automatic tick: %v", err)
	}
	if _, err := os.Stat(autoStatusPath(cfg)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no auto-update state write while updater is inactive, stat err=%v", err)
	}
	pending, err := pendingRequestPaths(cfg)
	if err != nil {
		t.Fatalf("pending request paths: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no queued updater request while runtime is inactive, got %v", pending)
	}
}

func TestCancelScheduledUpdateDefersCurrentVersion(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	mgr := NewManager(cfg)
	if err := os.MkdirAll(statusDir(cfg), 0o770); err != nil {
		t.Fatalf("mkdir status dir: %v", err)
	}
	rec := autoUpdateStateRecord{
		State:         AutoUpdateStateScheduled,
		TargetVersion: "v9.9.9",
		ScheduledFor:  time.Now().Add(2 * time.Hour).UTC(),
	}
	if err := writeJSONAtomic(autoStatusPath(cfg), rec, 0o640, 0o770); err != nil {
		t.Fatalf("write auto state: %v", err)
	}

	auto, err := mgr.CancelScheduledUpdate(context.Background(), st, "admin@example.com")
	if err != nil {
		t.Fatalf("cancel scheduled: %v", err)
	}
	if auto.State != AutoUpdateStateDownloaded {
		t.Fatalf("expected downloaded state after cancel, got %q", auto.State)
	}
	rec, err = readAutoUpdateState(autoStatusPath(cfg))
	if err != nil {
		t.Fatalf("read auto state: %v", err)
	}
	if rec.DeferredVersion != "v9.9.9" {
		t.Fatalf("expected deferred version recorded, got %q", rec.DeferredVersion)
	}
}

func TestRunWorkerEnsuresDirectoryContract(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{
		UpdateEnabled: true,
		UpdateBaseDir: filepath.Join(base, "update"),
	}
	mgr := NewManager(cfg)
	if err := mgr.runWorker(context.Background()); err != nil {
		t.Fatalf("run worker: %v", err)
	}
	if got := dirPerm(t, cfg.UpdateBaseDir); got != 0o750 {
		t.Fatalf("expected update base mode 0750, got %#o", got)
	}
	if got := dirPerm(t, requestDir(cfg)); got != 0o770 {
		t.Fatalf("expected request dir mode 0770, got %#o", got)
	}
	if got := dirPerm(t, statusDir(cfg)); got != 0o770 {
		t.Fatalf("expected status dir mode 0770, got %#o", got)
	}
	if got := dirPerm(t, lockDir(cfg)); got != 0o750 {
		t.Fatalf("expected lock dir mode 0750, got %#o", got)
	}
	if got := dirPerm(t, workDir(cfg)); got != 0o750 {
		t.Fatalf("expected work dir mode 0750, got %#o", got)
	}
	if got := dirPerm(t, backupsDir(cfg)); got != 0o750 {
		t.Fatalf("expected backups dir mode 0750, got %#o", got)
	}
}

func TestRunWorkerDiscardsInvalidRequestPayload(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{
		UpdateEnabled: true,
		UpdateBaseDir: filepath.Join(base, "update"),
	}
	if err := os.MkdirAll(requestDir(cfg), 0o770); err != nil {
		t.Fatalf("mkdir request dir: %v", err)
	}
	if err := os.WriteFile(requestPath(cfg), []byte("{"), 0o640); err != nil {
		t.Fatalf("write malformed request: %v", err)
	}

	mgr := NewManager(cfg)
	if err := mgr.runWorker(context.Background()); err != nil {
		t.Fatalf("run worker with malformed request: %v", err)
	}
	if _, err := os.Stat(requestPath(cfg)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected malformed request to be discarded, stat err=%v", err)
	}

	st, err := readApplyStatus(statusPath(cfg))
	if err != nil {
		t.Fatalf("read status after malformed request: %v", err)
	}
	if st.State != ApplyStateFailed {
		t.Fatalf("expected failed status for malformed request, got %q", st.State)
	}
	if !strings.Contains(st.Error, "invalid updater request payload") {
		t.Fatalf("unexpected malformed request status error: %q", st.Error)
	}
}

func TestRunWorkerRemovesStaleRequestWhenUpdatesDisabled(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{
		UpdateEnabled: false,
		UpdateBaseDir: filepath.Join(base, "update"),
	}
	if err := os.MkdirAll(requestDir(cfg), 0o770); err != nil {
		t.Fatalf("mkdir request dir: %v", err)
	}
	if err := os.WriteFile(requestPath(cfg), []byte(`{"request_id":"stale"}`), 0o640); err != nil {
		t.Fatalf("write stale request: %v", err)
	}

	mgr := NewManager(cfg)
	if err := mgr.runWorker(context.Background()); err != nil {
		t.Fatalf("run worker with updates disabled: %v", err)
	}
	if _, err := os.Stat(requestPath(cfg)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected stale request to be removed when updates are disabled, stat err=%v", err)
	}
}

func TestRunWorkerFromEnvSkipsConfigLoadWithoutPendingRequest(t *testing.T) {
	base := t.TempDir()
	t.Setenv("UPDATE_BASE_DIR", filepath.Join(base, "update"))
	t.Setenv("SESSION_ENCRYPT_KEY", "short")

	if err := RunWorkerFromEnv(context.Background()); err != nil {
		t.Fatalf("expected noise wakeup without pending request to exit cleanly, got %v", err)
	}
}

func TestRunWorkerFromEnvRequiresFullConfigWhenPendingRequestExists(t *testing.T) {
	base := t.TempDir()
	cfg := config.Config{
		UpdateBaseDir: filepath.Join(base, "update"),
	}
	if err := os.MkdirAll(requestDir(cfg), 0o770); err != nil {
		t.Fatalf("mkdir request dir: %v", err)
	}
	if err := os.WriteFile(requestPath(cfg), []byte(`{}`), 0o640); err != nil {
		t.Fatalf("write request: %v", err)
	}
	t.Setenv("UPDATE_BASE_DIR", cfg.UpdateBaseDir)
	t.Setenv("SESSION_ENCRYPT_KEY", "short")

	if err := RunWorkerFromEnv(context.Background()); err == nil {
		t.Fatalf("expected pending request to force full config load failure")
	}
}

func TestQueueApplyIgnoresUnreadableStatusFile(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	mgr := NewManager(cfg)
	if err := os.WriteFile(statusPath(cfg), []byte(`{"state":"idle"}`), 0o000); err != nil {
		t.Fatalf("write unreadable status file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(statusPath(cfg), 0o644)
	})

	if _, err := mgr.QueueApply(context.Background(), st, "admin@example.com", "v1.2.3", "req-perm"); err != nil {
		t.Fatalf("queue apply should tolerate unreadable status file: %v", err)
	}
}

func TestQueueApplyIgnoresUnreadableLockDir(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	makeLockDirUnreadable(t, cfg)
	mgr := NewManager(cfg)

	req, err := mgr.QueueApply(context.Background(), st, "admin@example.com", "v1.2.3", "req-lock-perms")
	if err != nil {
		t.Fatalf("queue apply with unreadable lock dir: %v", err)
	}
	if req.RequestID != "req-lock-perms" {
		t.Fatalf("unexpected request id: %q", req.RequestID)
	}
}

func TestStatusIgnoresUnreadableLockDirWhenIdle(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}
	makeLockDirUnreadable(t, cfg)
	mgr := NewManager(cfg)

	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status with unreadable lock dir: %v", err)
	}
	if status.Apply.State != ApplyStateIdle {
		t.Fatalf("expected idle apply state, got %#v", status.Apply)
	}
}

func TestStatusForceCheckIgnoresUnreadableLockDir(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/2high4schooltoday/despatch/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","name":"v1.2.3","published_at":"2026-01-01T00:00:00Z","html_url":"https://example.test/release","assets":[]}`))
	}))
	defer server.Close()
	makeLockDirUnreadable(t, cfg)
	mgr := NewManager(cfg)
	mgr.gh.baseURL = server.URL

	status, err := mgr.Status(context.Background(), st, true)
	if err != nil {
		t.Fatalf("forced status check with unreadable lock dir: %v", err)
	}
	if status.Latest == nil || status.Latest.TagName != "v1.2.3" {
		t.Fatalf("expected latest release from forced check, got %#v", status.Latest)
	}
	if status.Apply.State != ApplyStateIdle {
		t.Fatalf("expected idle apply state after forced check, got %#v", status.Apply)
	}
}

func TestStatusReportsUpdaterUnitMissingDiagnostic(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}
	mgr := NewManager(cfg)
	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Configured {
		t.Fatalf("expected configured=false when updater marker is missing")
	}
	if status.ConfigDiagnostic == nil {
		t.Fatalf("expected config diagnostic when updater marker is missing")
	}
	if status.ConfigDiagnostic.Reason != "updater_unit_missing" {
		t.Fatalf("expected updater_unit_missing, got %q", status.ConfigDiagnostic.Reason)
	}
}

func TestStatusReportsRequestDirDiagnostic(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := os.MkdirAll(cfg.UpdateBaseDir, 0o755); err != nil {
		t.Fatalf("mkdir update base dir: %v", err)
	}
	if err := os.WriteFile(requestDir(cfg), []byte("block dir creation"), 0o644); err != nil {
		t.Fatalf("write request dir blocker: %v", err)
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}
	mgr := NewManager(cfg)
	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Configured {
		t.Fatalf("expected configured=false when request path is not a directory")
	}
	if status.ConfigDiagnostic == nil {
		t.Fatalf("expected request dir diagnostic")
	}
	if status.ConfigDiagnostic.Reason != "request_dir_unwritable" {
		t.Fatalf("expected request_dir_unwritable, got %q", status.ConfigDiagnostic.Reason)
	}
	if !strings.Contains(status.ConfigDiagnostic.RepairHint, "install -d -o root -g despatch -m 0770") {
		t.Fatalf("expected repair hint to include ownership/mode fix command, got %q", status.ConfigDiagnostic.RepairHint)
	}
}

func TestStatusReportsRequestDirDiagnosticWithoutMutatingPermissions(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := os.MkdirAll(requestDir(cfg), 0o755); err != nil {
		t.Fatalf("mkdir request dir: %v", err)
	}
	if err := os.Chmod(requestDir(cfg), 0o500); err != nil {
		t.Fatalf("chmod request dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(requestDir(cfg), 0o755)
	})
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}
	mgr := NewManager(cfg)
	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Configured {
		t.Fatalf("expected configured=false when request dir is not writable")
	}
	if status.ConfigDiagnostic == nil {
		t.Fatalf("expected request dir diagnostic")
	}
	if status.ConfigDiagnostic.Reason != "request_dir_unwritable" {
		t.Fatalf("expected request_dir_unwritable, got %#v", status.ConfigDiagnostic)
	}
	if got := dirPerm(t, requestDir(cfg)); got != 0o500 {
		t.Fatalf("expected request dir mode to remain unchanged at 0500, got %#o", got)
	}
}

func TestStatusConfiguredWhenUpdaterReady(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}
	mgr := NewManager(cfg)
	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Configured {
		t.Fatalf("expected configured=true when updater marker and writable paths are present")
	}
	if status.ConfigDiagnostic != nil {
		t.Fatalf("expected no config diagnostic when updater is configured, got %#v", status.ConfigDiagnostic)
	}
}

func TestStatusConfiguredWhenUpdaterReadyDoesNotMutateWatchedDirectories(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	frozen := time.Unix(1_700_000_000, 0).UTC()
	if err := os.Chtimes(requestDir(cfg), frozen, frozen); err != nil {
		t.Fatalf("chtimes request dir: %v", err)
	}
	if err := os.Chtimes(statusDir(cfg), frozen, frozen); err != nil {
		t.Fatalf("chtimes status dir: %v", err)
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}
	mgr := NewManager(cfg)

	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !status.Configured {
		t.Fatalf("expected configured=true when updater is ready")
	}
	requestInfo, err := os.Stat(requestDir(cfg))
	if err != nil {
		t.Fatalf("stat request dir: %v", err)
	}
	statusInfo, err := os.Stat(statusDir(cfg))
	if err != nil {
		t.Fatalf("stat status dir: %v", err)
	}
	if !requestInfo.ModTime().Equal(frozen) {
		t.Fatalf("expected request dir modtime to stay %s, got %s", frozen, requestInfo.ModTime())
	}
	if !statusInfo.ModTime().Equal(frozen) {
		t.Fatalf("expected status dir modtime to stay %s, got %s", frozen, statusInfo.ModTime())
	}
}

func TestStatusReportsQueuedWhenRequestFileIsPending(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	now := time.Now().UTC()
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	req := ApplyRequest{
		RequestID:     "req-pending",
		RequestedAt:   now,
		RequestedBy:   "admin@example.com",
		TargetVersion: "v1.2.3",
	}
	if err := writeJSONAtomic(requestQueuePath(req, cfg), req, 0o640, updaterDirModeForPath(cfg, requestDir(cfg), 0o750)); err != nil {
		t.Fatalf("write queued request: %v", err)
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}
	makeLockDirUnreadable(t, cfg)
	mgr := NewManager(cfg)

	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status with pending request: %v", err)
	}
	if status.Apply.State != ApplyStateQueued {
		t.Fatalf("expected queued apply state from pending request, got %#v", status.Apply)
	}
}

func TestStatusReportsUpdaterPathInactiveDiagnostic(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "inactive", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}
	mgr := NewManager(cfg)
	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Configured {
		t.Fatalf("expected configured=false when updater path unit is inactive")
	}
	if status.ConfigDiagnostic == nil || status.ConfigDiagnostic.Reason != "updater_path_inactive" {
		t.Fatalf("expected updater_path_inactive diagnostic, got %#v", status.ConfigDiagnostic)
	}
}

func TestStatusReportsUpdaterPathTriggerLimitDiagnostic(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctlWithDetails(t, "loaded", "failed", "failed", "unit-start-limit-hit", "loaded", "failed", "failed", "exit-code")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}
	mgr := NewManager(cfg)
	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Configured {
		t.Fatalf("expected configured=false when updater path unit hit trigger limit")
	}
	if status.ConfigDiagnostic == nil || status.ConfigDiagnostic.Reason != "updater_path_trigger_limited" {
		t.Fatalf("expected updater_path_trigger_limited diagnostic, got %#v", status.ConfigDiagnostic)
	}
	if !strings.Contains(status.ConfigDiagnostic.RepairHint, "reset-failed") {
		t.Fatalf("expected trigger-limit repair hint to include reset-failed, got %q", status.ConfigDiagnostic.RepairHint)
	}
}

func TestQueueApplyWritesUniqueRequestFile(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	mgr := NewManager(cfg)
	req, err := mgr.QueueApply(context.Background(), st, "admin@example.com", "v1.2.3", "req-queue")
	if err != nil {
		t.Fatalf("queue apply: %v", err)
	}
	pending, err := pendingRequestPaths(cfg)
	if err != nil {
		t.Fatalf("pending request paths: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected exactly one pending request path, got %v", pending)
	}
	if filepath.Base(pending[0]) == filepath.Base(requestPath(cfg)) {
		t.Fatalf("expected non-legacy queued request file, got %s", pending[0])
	}
	var decoded ApplyRequest
	if err := readJSONFile(pending[0], &decoded); err != nil {
		t.Fatalf("read queued request: %v", err)
	}
	if decoded.RequestID != req.RequestID {
		t.Fatalf("expected queued request id %q, got %q", req.RequestID, decoded.RequestID)
	}
}

func TestStatusFailsStaleQueuedRequest(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "inactive", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	mgr := NewManager(cfg)
	now := time.Now().UTC()
	mgr.now = func() time.Time { return now }
	req := ApplyRequest{
		RequestID:     "req-stale",
		RequestedAt:   now.Add(-2 * updateQueuePickupGrace),
		RequestedBy:   "admin@example.com",
		TargetVersion: "v1.2.3",
	}
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	if err := writeJSONAtomic(requestQueuePath(req, cfg), req, 0o640, updaterDirModeForPath(cfg, requestDir(cfg), 0o750)); err != nil {
		t.Fatalf("write request file: %v", err)
	}
	if err := writeJSONAtomic(statusPath(cfg), ApplyStatus{
		State:         ApplyStateQueued,
		RequestID:     req.RequestID,
		RequestedAt:   req.RequestedAt,
		TargetVersion: req.TargetVersion,
	}, 0o640, updaterDirModeForPath(cfg, statusDir(cfg), 0o750)); err != nil {
		t.Fatalf("write status file: %v", err)
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}
	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Apply.State != ApplyStateFailed {
		t.Fatalf("expected stale queued request to become failed, got %#v", status.Apply)
	}
	if !strings.Contains(status.Apply.Error, "queued request was not picked up") {
		t.Fatalf("unexpected stale queue error: %q", status.Apply.Error)
	}
	pending, err := pendingRequestPaths(cfg)
	if err != nil {
		t.Fatalf("pending request paths: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected stale request files to remain untouched during status read, got %v", pending)
	}
	stored, err := readApplyStatus(statusPath(cfg))
	if err != nil {
		t.Fatalf("read stored apply status: %v", err)
	}
	if stored.State != ApplyStateQueued {
		t.Fatalf("expected stored apply status to remain queued during status read, got %#v", stored)
	}
}

func TestStatusFailsStaleQueuedRequestWhenLockDirUnreadable(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "inactive", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	now := time.Now().UTC()
	mgr := NewManager(cfg)
	mgr.now = func() time.Time { return now }
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	makeLockDirUnreadable(t, cfg)
	req := ApplyRequest{
		RequestID:     "req-stale-unreadable-lock",
		RequestedAt:   now.Add(-2 * updateQueuePickupGrace),
		RequestedBy:   "admin@example.com",
		TargetVersion: "v1.2.3",
	}
	if err := writeJSONAtomic(requestQueuePath(req, cfg), req, 0o640, updaterDirModeForPath(cfg, requestDir(cfg), 0o750)); err != nil {
		t.Fatalf("write request file: %v", err)
	}
	if err := writeJSONAtomic(statusPath(cfg), ApplyStatus{
		State:         ApplyStateQueued,
		RequestID:     req.RequestID,
		RequestedAt:   req.RequestedAt,
		TargetVersion: req.TargetVersion,
	}, 0o640, updaterDirModeForPath(cfg, statusDir(cfg), 0o750)); err != nil {
		t.Fatalf("write status file: %v", err)
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}

	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status with unreadable lock dir: %v", err)
	}
	if status.Apply.State != ApplyStateFailed {
		t.Fatalf("expected stale queued request to become failed, got %#v", status.Apply)
	}
	if !strings.Contains(status.Apply.Error, "queued request was not picked up") {
		t.Fatalf("unexpected stale queue error: %q", status.Apply.Error)
	}
	pending, err := pendingRequestPaths(cfg)
	if err != nil {
		t.Fatalf("pending request paths: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected stale request files to remain untouched during status read, got %v", pending)
	}
}

func TestStatusRecoversStalePreparingAutoUpdate(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	now := time.Now().UTC()
	mgr := NewManager(cfg)
	mgr.now = func() time.Time { return now }
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	if err := writeAutoUpdateState(cfg, autoUpdateStateRecord{
		State:         AutoUpdateStatePreparing,
		TargetVersion: "v1.2.3",
	}); err != nil {
		t.Fatalf("write auto state: %v", err)
	}
	req := ApplyRequest{
		RequestID:     "auto-prepare-stale",
		RequestedAt:   now.Add(-2 * updateQueuePickupGrace),
		RequestedBy:   "system:auto-update",
		Mode:          ApplyModePrepare,
		TargetVersion: "v1.2.3",
	}
	if err := writeJSONAtomic(requestQueuePath(req, cfg), req, 0o640, updaterDirModeForPath(cfg, requestDir(cfg), 0o750)); err != nil {
		t.Fatalf("write prepare request: %v", err)
	}
	if err := st.UpsertSetting(context.Background(), settingLastCheckAt, now.Format(time.RFC3339)); err != nil {
		t.Fatalf("set last check timestamp: %v", err)
	}

	status, err := mgr.Status(context.Background(), st, false)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.AutoUpdate.State != AutoUpdateStateFailed {
		t.Fatalf("expected stale preparing state to become failed, got %#v", status.AutoUpdate)
	}
	if !strings.Contains(status.AutoUpdate.Error, "queued request was not picked up") {
		t.Fatalf("unexpected stale prepare error: %q", status.AutoUpdate.Error)
	}
	pending, err := pendingRequestPaths(cfg)
	if err != nil {
		t.Fatalf("pending request paths: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected stale prepare request to be cleared, got %v", pending)
	}
	stored, err := readAutoUpdateState(autoStatusPath(cfg))
	if err != nil {
		t.Fatalf("read stored auto state: %v", err)
	}
	if stored.State != AutoUpdateStateFailed {
		t.Fatalf("expected stored auto state to become failed, got %#v", stored)
	}
}

func TestQueueApplyRecoversStalePreparingAutoUpdate(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")
	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
	}
	now := time.Now().UTC()
	mgr := NewManager(cfg)
	mgr.now = func() time.Time { return now }
	if err := ensureUpdaterRequestStatusDirectories(cfg); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	if err := writeAutoUpdateState(cfg, autoUpdateStateRecord{
		State:         AutoUpdateStatePreparing,
		TargetVersion: "v1.2.3",
	}); err != nil {
		t.Fatalf("write auto state: %v", err)
	}
	req := ApplyRequest{
		RequestID:     "auto-prepare-stale",
		RequestedAt:   now.Add(-2 * updateQueuePickupGrace),
		RequestedBy:   "system:auto-update",
		Mode:          ApplyModePrepare,
		TargetVersion: "v1.2.3",
	}
	if err := writeJSONAtomic(requestQueuePath(req, cfg), req, 0o640, updaterDirModeForPath(cfg, requestDir(cfg), 0o750)); err != nil {
		t.Fatalf("write prepare request: %v", err)
	}

	applyReq, err := mgr.QueueApply(context.Background(), st, "admin@example.com", "v1.2.3", "manual-apply")
	if err != nil {
		t.Fatalf("queue manual apply: %v", err)
	}
	if applyReq.RequestID != "manual-apply" {
		t.Fatalf("unexpected apply request id: %q", applyReq.RequestID)
	}
	pending, err := pendingRequests(cfg)
	if err != nil {
		t.Fatalf("pending requests: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected only queued manual apply request to remain, got %#v", pending)
	}
	if pending[0].Request.Mode != ApplyModeApply {
		t.Fatalf("expected manual apply request mode, got %q", pending[0].Request.Mode)
	}
	stored, err := readAutoUpdateState(autoStatusPath(cfg))
	if err != nil {
		t.Fatalf("read stored auto state: %v", err)
	}
	if stored.State != AutoUpdateStateFailed {
		t.Fatalf("expected stale auto prepare to be cleared before manual apply, got %#v", stored)
	}
}

func TestMigrateLegacyPasswordResetEnvRewritesBrokenLoopbackResetSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	raw := strings.Join([]string{
		"BASE_DOMAIN=mail.2h4s2d.ru",
		"DEPLOY_MODE=proxy",
		"PROXY_SERVER_NAME=mail.2h4s2d.ru",
		"PROXY_TLS=1",
		"LISTEN_ADDR=127.0.0.1:8080",
		"SMTP_HOST=127.0.0.1",
		"SMTP_PORT=587",
		"SMTP_TLS=false",
		"SMTP_STARTTLS=true",
		"PASSWORD_RESET_SENDER=smtp",
		"PASSWORD_RESET_FROM=no-reply@mail.2h4s2d.ru",
		"PASSWORD_RESET_BASE_URL=",
		"DOVECOT_AUTH_MODE=pam",
		"PASSWORD_RESET_EXTERNAL_SENDER_READY=false",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(raw), 0o640); err != nil {
		t.Fatalf("write env: %v", err)
	}
	migrated, err := migrateLegacyPasswordResetEnv(path)
	if err != nil {
		t.Fatalf("migrate legacy env: %v", err)
	}
	if !migrated {
		t.Fatalf("expected migration to run")
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read updated env: %v", err)
	}
	text := string(updated)
	for _, want := range []string{
		"SMTP_PORT=25",
		"SMTP_STARTTLS=false",
		"SMTP_TLS=false",
		"PASSWORD_RESET_FROM=no-reply@2h4s2d.ru",
		"PASSWORD_RESET_BASE_URL=https://mail.2h4s2d.ru",
		"PASSWORD_RESET_EXTERNAL_SENDER_READY=true",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected updated env to contain %q, got:\n%s", want, text)
		}
	}
}

func TestMigrateLegacyPasswordResetEnvSkipsCustomSMTPSetup(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	raw := strings.Join([]string{
		"BASE_DOMAIN=mail.2h4s2d.ru",
		"SMTP_HOST=smtp.example.net",
		"SMTP_PORT=587",
		"SMTP_TLS=false",
		"SMTP_STARTTLS=true",
		"PASSWORD_RESET_SENDER=smtp",
		"PASSWORD_RESET_FROM=ops@example.net",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(raw), 0o640); err != nil {
		t.Fatalf("write env: %v", err)
	}
	migrated, err := migrateLegacyPasswordResetEnv(path)
	if err != nil {
		t.Fatalf("migrate legacy env: %v", err)
	}
	if migrated {
		t.Fatalf("expected custom SMTP setup to be left alone")
	}
}

func TestFindPayloadRootRequiresDeploy(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"despatch", "despatch-pam-reset-helper", "despatch-update-worker"} {
		if err := os.WriteFile(filepath.Join(root, rel), []byte("ok"), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	for _, rel := range []string{"web", "migrations"} {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
	}
	if _, err := findPayloadRoot(root); err == nil {
		t.Fatalf("expected missing deploy directory to fail payload root validation")
	}
	if err := os.MkdirAll(filepath.Join(root, "deploy"), 0o755); err != nil {
		t.Fatalf("mkdir deploy: %v", err)
	}
	if _, err := findPayloadRoot(root); err != nil {
		t.Fatalf("expected payload root validation to pass once deploy exists: %v", err)
	}
}

func TestGitHubLatestReleaseWithETag(t *testing.T) {
	t.Parallel()
	serverETag := `"abc123"`
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/2high4schooltoday/despatch/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == serverETag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", serverETag)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","name":"v1.2.3","published_at":"2026-01-01T00:00:00Z","html_url":"https://example.test/release","assets":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := config.Config{
		UpdateRepoOwner:      "2high4schooltoday",
		UpdateRepoName:       "despatch",
		UpdateHTTPTimeoutSec: 5,
	}
	gh := newGitHubClient(cfg)
	gh.baseURL = srv.URL

	first, etag, notModified, err := gh.latestRelease(context.Background(), "")
	if err != nil {
		t.Fatalf("first latest release call: %v", err)
	}
	if notModified {
		t.Fatalf("did not expect not-modified on first request")
	}
	if first.TagName != "v1.2.3" {
		t.Fatalf("unexpected tag: %q", first.TagName)
	}
	second, _, notModified, err := gh.latestRelease(context.Background(), etag)
	if err != nil {
		t.Fatalf("second latest release call: %v", err)
	}
	if !notModified {
		t.Fatalf("expected not-modified on second request")
	}
	if second.TagName != "" {
		t.Fatalf("expected empty release payload on 304, got %q", second.TagName)
	}
}

func TestGitHubLatestReleaseFallsBackToPrereleaseWhenLatest404(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/2high4schooltoday/despatch/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/repos/2high4schooltoday/despatch/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"tag_name":"v1.0.0-alpha.1.1","name":"Alpha 1.0.0_01","published_at":"2026-03-01T12:29:38Z","html_url":"https://example.test/release-1","draft":false,"prerelease":true,"assets":[]},
			{"tag_name":"v1.0.0-alpha.1","name":"Alpha 1.0.0","published_at":"2026-03-01T12:21:49Z","html_url":"https://example.test/release-2","draft":false,"prerelease":true,"assets":[]}
		]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := config.Config{
		UpdateRepoOwner:      "2high4schooltoday",
		UpdateRepoName:       "despatch",
		UpdateHTTPTimeoutSec: 5,
	}
	gh := newGitHubClient(cfg)
	gh.baseURL = srv.URL

	latest, _, notModified, err := gh.latestRelease(context.Background(), "")
	if err != nil {
		t.Fatalf("latest release fallback call: %v", err)
	}
	if notModified {
		t.Fatalf("did not expect not-modified from fallback call")
	}
	if latest.TagName != "v1.0.0-alpha.1.1" {
		t.Fatalf("unexpected fallback tag: %q", latest.TagName)
	}
}

func TestArchiveAssetCandidatesIncludeArchAliases(t *testing.T) {
	t.Parallel()
	candidates := archiveAssetCandidates("arm64")
	joined := strings.Join(candidates, ",")
	if !strings.Contains(joined, "despatch-linux-arm64.tar.gz") {
		t.Fatalf("missing arm64 archive candidate: %v", candidates)
	}
	if !strings.Contains(joined, "despatch-linux-aarch64.tar.gz") {
		t.Fatalf("missing aarch64 alias candidate: %v", candidates)
	}
}

func TestResolveArchiveAssetUsesArchAlias(t *testing.T) {
	t.Parallel()
	release := githubRelease{
		Assets: []githubReleaseAsset{
			{Name: "despatch-linux-aarch64.tar.gz", URL: "https://example.test/aarch64.tar.gz"},
			{Name: "checksums.txt", URL: "https://example.test/checksums.txt"},
		},
	}
	name, url, ok := resolveArchiveAsset(release, "arm64")
	if !ok {
		t.Fatalf("expected alias archive resolution to succeed")
	}
	if name != "despatch-linux-aarch64.tar.gz" {
		t.Fatalf("unexpected archive name: %q", name)
	}
	if url != "https://example.test/aarch64.tar.gz" {
		t.Fatalf("unexpected archive url: %q", url)
	}
}

func TestResolveArchiveAssetMissing(t *testing.T) {
	t.Parallel()
	release := githubRelease{
		Assets: []githubReleaseAsset{
			{Name: "checksums.txt", URL: "https://example.test/checksums.txt"},
		},
	}
	if _, _, ok := resolveArchiveAsset(release, "arm64"); ok {
		t.Fatalf("expected missing archive lookup to fail")
	}
}

func TestIsReadOnlyOrPermissionError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "permission", err: os.ErrPermission, want: true},
		{name: "read_only_fs", err: &os.PathError{Op: "open", Path: "/etc/systemd/system/despatch-mailsec.service", Err: syscall.EROFS}, want: true},
		{name: "other", err: errors.New("boom"), want: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isReadOnlyOrPermissionError(tt.err)
			if got != tt.want {
				t.Fatalf("isReadOnlyOrPermissionError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsSystemdLoadStateKnown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state string
		want  bool
	}{
		{state: "", want: false},
		{state: "not-found", want: false},
		{state: "error", want: false},
		{state: "bad-setting", want: false},
		{state: "loaded", want: true},
		{state: "masked", want: true},
		{state: "generated", want: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.state, func(t *testing.T) {
			t.Parallel()
			if got := isSystemdLoadStateKnown(tt.state); got != tt.want {
				t.Fatalf("isSystemdLoadStateKnown(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestVerifyChecksumSignature(t *testing.T) {
	t.Parallel()
	pub, priv, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate keypair: %v", err)
	}
	otherPub, _, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		t.Fatalf("generate secondary keypair: %v", err)
	}

	tmp := t.TempDir()
	checksumPath := filepath.Join(tmp, "checksums.txt")
	if err := os.WriteFile(checksumPath, []byte("abc123  despatch-linux-amd64.tar.gz\n"), 0o640); err != nil {
		t.Fatalf("write checksum: %v", err)
	}

	sig := ed25519.Sign(priv, []byte("abc123  despatch-linux-amd64.tar.gz\n"))
	sigPath := filepath.Join(tmp, "checksums.txt.sig")
	if err := os.WriteFile(sigPath, sig, 0o640); err != nil {
		t.Fatalf("write signature: %v", err)
	}
	pubKey := base64.StdEncoding.EncodeToString(pub)
	if err := verifyChecksumSignature(checksumPath, sigPath, []string{pubKey}); err != nil {
		t.Fatalf("expected signature verification success, got: %v", err)
	}

	if err := os.WriteFile(checksumPath, []byte("tampered\n"), 0o640); err != nil {
		t.Fatalf("write tampered checksum: %v", err)
	}
	if err := verifyChecksumSignature(checksumPath, sigPath, []string{pubKey}); err == nil {
		t.Fatalf("expected verification to fail for tampered checksum")
	}

	if err := os.WriteFile(checksumPath, []byte("abc123  despatch-linux-amd64.tar.gz\n"), 0o640); err != nil {
		t.Fatalf("restore checksum: %v", err)
	}
	otherKey := base64.StdEncoding.EncodeToString(otherPub)
	if err := verifyChecksumSignature(checksumPath, sigPath, []string{otherKey}); err == nil {
		t.Fatalf("expected verification to fail with wrong key")
	}

	if err := os.WriteFile(sigPath, []byte(base64.StdEncoding.EncodeToString(sig)), 0o640); err != nil {
		t.Fatalf("write base64 signature: %v", err)
	}
	if err := verifyChecksumSignature(checksumPath, sigPath, []string{pubKey}); err != nil {
		t.Fatalf("expected base64 signature verification success, got: %v", err)
	}
}

func TestNormalizeRuntimeTreePermissionsMakesDeployedTreeReadable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	lockedDir := filepath.Join(root, "migrations")
	nestedDir := filepath.Join(lockedDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o750); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	plainFile := filepath.Join(nestedDir, "001_init.sql")
	if err := os.WriteFile(plainFile, []byte("select 1;\n"), 0o640); err != nil {
		t.Fatalf("write plain file: %v", err)
	}
	execFile := filepath.Join(root, "tool.sh")
	if err := os.WriteFile(execFile, []byte("#!/bin/sh\nexit 0\n"), 0o750); err != nil {
		t.Fatalf("write exec file: %v", err)
	}

	if err := normalizeRuntimeTreePermissions(root); err != nil {
		t.Fatalf("normalizeRuntimeTreePermissions: %v", err)
	}

	assertMode := func(path string, want os.FileMode) {
		t.Helper()
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("mode for %s = %#o want %#o", path, got, want)
		}
	}

	assertMode(root, 0o755)
	assertMode(lockedDir, 0o755)
	assertMode(nestedDir, 0o755)
	assertMode(plainFile, 0o644)
	assertMode(execFile, 0o755)
}

func TestMovePathWithCrossDeviceFallbackMovesRegularFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "backup", "src.txt")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	if err := os.WriteFile(src, []byte("hello\n"), 0o640); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := movePathWithCrossDeviceFallback(src, dst); err != nil {
		t.Fatalf("movePathWithCrossDeviceFallback: %v", err)
	}

	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected src to be gone, stat err=%v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "hello\n" {
		t.Fatalf("unexpected dst contents %q", got)
	}
}

func TestMovePathWithCrossDeviceFallbackMovesDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	src := filepath.Join(root, "srcdir")
	dst := filepath.Join(root, "backup", "srcdir")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("mkdir backup dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(src, "nested"), 0o750); err != nil {
		t.Fatalf("mkdir srcdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "nested", "data.txt"), []byte("ok\n"), 0o640); err != nil {
		t.Fatalf("write nested file: %v", err)
	}

	if err := movePathWithCrossDeviceFallback(src, dst); err != nil {
		t.Fatalf("movePathWithCrossDeviceFallback dir: %v", err)
	}

	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected src dir to be gone, stat err=%v", err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "nested", "data.txt"))
	if err != nil {
		t.Fatalf("read moved nested file: %v", err)
	}
	if string(got) != "ok\n" {
		t.Fatalf("unexpected moved nested contents %q", got)
	}
}

func TestMovePathWithCrossDeviceFallbackMissingSourceIsNoop(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	src := filepath.Join(root, "missing.txt")
	dst := filepath.Join(root, "backup", "missing.txt")
	if err := movePathWithCrossDeviceFallback(src, dst); err != nil {
		t.Fatalf("expected missing source to be ignored, got %v", err)
	}
}

func TestMovePathWithCrossDeviceFallbackMissingDestinationParentReturnsError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "missing-parent", "src.txt")
	if err := os.WriteFile(src, []byte("hello\n"), 0o640); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := movePathWithCrossDeviceFallback(src, dst); err == nil {
		t.Fatal("expected movePathWithCrossDeviceFallback to fail when destination parent is missing")
	}
}

func TestApplyReleaseTreatsBackupArchiveFailureAsWarning(t *testing.T) {
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	writeUpdaterUnitFiles(t, unitDir)
	installFakeSystemctl(t, "loaded", "active", "loaded", "inactive")

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/2high4schooltoday/despatch/releases/tags/v9.9.9", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9","name":"v9.9.9","published_at":"2026-03-10T00:00:00Z","html_url":"https://example.test/release","assets":[]}`))
	})
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/v1/setup/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cfg := config.Config{
		UpdateEnabled:          true,
		UpdateRepoOwner:        "2high4schooltoday",
		UpdateRepoName:         "despatch",
		UpdateCheckIntervalMin: 60,
		UpdateHTTPTimeoutSec:   10,
		UpdateBackupKeep:       3,
		UpdateBaseDir:          filepath.Join(base, "update"),
		UpdateInstallDir:       filepath.Join(base, "install"),
		UpdateServiceName:      "despatch",
		UpdateSystemdUnitDir:   unitDir,
		ListenAddr:             strings.TrimPrefix(srv.URL, "http://"),
	}
	mgr := NewManager(cfg)
	mgr.gh.baseURL = srv.URL

	writeTree := func(root, version string) {
		t.Helper()
		for _, name := range []string{"despatch", "despatch-pam-reset-helper", "despatch-update-worker"} {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
				t.Fatalf("mkdir %s parent: %v", name, err)
			}
			if err := os.WriteFile(filepath.Join(root, name), []byte(version+"-"+name+"\n"), 0o755); err != nil {
				t.Fatalf("write %s: %v", name, err)
			}
		}
		for _, name := range []string{"web", "migrations", "deploy"} {
			dir := filepath.Join(root, name)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", name, err)
			}
			if err := os.WriteFile(filepath.Join(dir, "version.txt"), []byte(version+"-"+name+"\n"), 0o644); err != nil {
				t.Fatalf("write %s marker: %v", name, err)
			}
		}
		deployDir := filepath.Join(root, "deploy")
		serviceUnit := fmt.Sprintf("[Service]\nExecStart=%s\n", filepath.Join(cfg.UpdateInstallDir, "despatch-update-worker"))
		if err := os.WriteFile(filepath.Join(deployDir, "despatch-updater.service"), []byte(serviceUnit), 0o644); err != nil {
			t.Fatalf("write deploy updater service: %v", err)
		}
		if err := os.WriteFile(filepath.Join(deployDir, "despatch-updater.path"), []byte("[Path]\nUnit=despatch-updater.service\n"), 0o644); err != nil {
			t.Fatalf("write deploy updater path: %v", err)
		}
	}

	writeTree(cfg.UpdateInstallDir, "old")
	writeTree(preparedPayloadDir(cfg, "v9.9.9"), "new")

	if err := os.MkdirAll(cfg.UpdateBaseDir, 0o750); err != nil {
		t.Fatalf("mkdir update base: %v", err)
	}
	if err := os.WriteFile(backupsDir(cfg), []byte("block backup dir creation"), 0o640); err != nil {
		t.Fatalf("write backup blocker: %v", err)
	}

	toVersion, rolledBack, err := mgr.applyRelease(context.Background(), ApplyRequest{
		RequestID:     "req-backup-warning",
		TargetVersion: "v9.9.9",
	})
	if err != nil {
		t.Fatalf("applyRelease: %v", err)
	}
	if rolledBack {
		t.Fatalf("expected no rollback when backup archive fails")
	}
	if toVersion != "v9.9.9" {
		t.Fatalf("expected toVersion v9.9.9, got %q", toVersion)
	}

	assertContains := func(path, want string) {
		t.Helper()
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(got), want) {
			t.Fatalf("expected %s to contain %q, got %q", path, want, string(got))
		}
	}

	assertContains(filepath.Join(cfg.UpdateInstallDir, "despatch"), "new-despatch")
	assertContains(filepath.Join(cfg.UpdateInstallDir, "web", "version.txt"), "new-web")
	if _, err := os.Stat(filepath.Join(cfg.UpdateInstallDir, ".prev-despatch-req-backup-warning")); err != nil {
		t.Fatalf("expected previous binary to remain for later cleanup when backup archive fails, stat err=%v", err)
	}
}
