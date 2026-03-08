package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/store"
	"despatch/internal/version"
)

func RunWorker(ctx context.Context, cfg config.Config) error {
	mgr := NewManager(cfg)
	return mgr.runWorker(ctx)
}

func (m *Manager) runWorker(ctx context.Context) error {
	if !m.cfg.UpdateEnabled {
		// Prevent a stale request file from repeatedly retriggering the path unit
		// when updates are intentionally disabled.
		_ = removePendingRequestPaths(m.cfg)
		return nil
	}
	if err := ensureUpdaterRuntimeDirectories(m.cfg); err != nil {
		return err
	}
	lockFD, err := os.OpenFile(lockPath(m.cfg), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	_, _ = lockFD.WriteString(fmt.Sprintf("pid=%d started_at=%s\n", os.Getpid(), m.now().Format(time.RFC3339)))
	_ = lockFD.Close()
	defer os.Remove(lockPath(m.cfg))

	reqPath, err := firstPendingRequestPath(m.cfg)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	var req ApplyRequest
	if err := readJSONFile(reqPath, &req); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if isInvalidApplyRequestPayloadError(err) {
			now := m.now()
			_ = os.Remove(reqPath)
			_ = writeJSONAtomic(statusPath(m.cfg), ApplyStatus{
				State:       ApplyStateFailed,
				RequestID:   fmt.Sprintf("invalid-request-%d", now.Unix()),
				RequestedAt: now,
				StartedAt:   now,
				FinishedAt:  now,
				FromVersion: version.Current().Version,
				Error:       "invalid updater request payload; discarded",
			}, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750))
			_ = ensureDespatchReadable(statusPath(m.cfg))
			return nil
		}
		return err
	}
	if strings.TrimSpace(req.RequestID) == "" {
		req.RequestID = fmt.Sprintf("update-%d", m.now().Unix())
	}
	if req.RequestedAt.IsZero() {
		req.RequestedAt = m.now()
	}

	started := m.now()
	_ = writeJSONAtomic(statusPath(m.cfg), ApplyStatus{
		State:         ApplyStateInProgress,
		RequestID:     req.RequestID,
		RequestedAt:   req.RequestedAt,
		StartedAt:     started,
		TargetVersion: strings.TrimSpace(req.TargetVersion),
		FromVersion:   version.Current().Version,
	}, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750))
	_ = ensureDespatchReadable(statusPath(m.cfg))
	if err := os.Remove(reqPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	finalStatus := ApplyStatus{
		State:         ApplyStateFailed,
		RequestID:     req.RequestID,
		RequestedAt:   req.RequestedAt,
		StartedAt:     started,
		FinishedAt:    m.now(),
		TargetVersion: strings.TrimSpace(req.TargetVersion),
		FromVersion:   version.Current().Version,
	}
	defer func() {
		finalStatus.FinishedAt = m.now()
		_ = writeJSONAtomic(statusPath(m.cfg), finalStatus, 0o640, updaterDirModeForPath(m.cfg, statusDir(m.cfg), 0o750))
		_ = ensureDespatchReadable(statusPath(m.cfg))
		_ = os.Remove(reqPath)
	}()

	target := strings.TrimSpace(req.TargetVersion)
	if target != "" && !targetVersionRx.MatchString(target) {
		finalStatus.Error = ErrInvalidTargetVersion.Error()
		return ErrInvalidTargetVersion
	}
	toVersion, rollback, err := m.applyRelease(ctx, req)
	if err != nil {
		finalStatus.Error = err.Error()
		finalStatus.ToVersion = toVersion
		finalStatus.RolledBack = rollback
		if rollback {
			finalStatus.State = ApplyStateRolledBack
		}
		return err
	}
	finalStatus.State = ApplyStateCompleted
	finalStatus.ToVersion = toVersion

	if sqdb, err := db.OpenSQLite(m.cfg.DBPath, 1, 1, time.Minute); err == nil {
		st := store.New(sqdb)
		m.MarkSuccess(context.Background(), st, toVersion)
		_ = sqdb.Close()
	}
	return nil
}

func isInvalidApplyRequestPayloadError(err error) bool {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &syntaxErr) ||
		errors.As(err, &typeErr) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

func (m *Manager) applyRelease(ctx context.Context, req ApplyRequest) (string, bool, error) {
	release, err := m.resolveRelease(ctx, strings.TrimSpace(req.TargetVersion))
	if err != nil {
		return "", false, err
	}
	arch := runtime.GOARCH
	archiveName, archiveURL, ok := resolveArchiveAsset(release, arch)
	if !ok {
		candidates := strings.Join(archiveAssetCandidates(arch), ", ")
		return "", false, fmt.Errorf(
			"release archive for GOARCH=%s not found (expected one of: %s); available assets: %s",
			arch,
			candidates,
			strings.Join(releaseAssetNames(release), ", "),
		)
	}
	checksumName := "checksums.txt"
	checksumURL, ok := findAssetURL(release, checksumName)
	if !ok {
		return "", false, fmt.Errorf("release asset %s not found", checksumName)
	}
	signatureName := strings.TrimSpace(m.cfg.UpdateSignatureAsset)
	signatureURL := ""
	if m.cfg.UpdateRequireSignature {
		if signatureName == "" {
			return "", false, fmt.Errorf("UPDATE_SIGNATURE_ASSET is required when UPDATE_REQUIRE_SIGNATURE=true")
		}
		var found bool
		signatureURL, found = findAssetURL(release, signatureName)
		if !found {
			return "", false, fmt.Errorf("release asset %s not found", signatureName)
		}
	}

	runID := req.RequestID
	if strings.TrimSpace(runID) == "" {
		runID = fmt.Sprintf("run-%d", m.now().Unix())
	}
	runWork := filepath.Join(workDir(m.cfg), sanitizePathToken(runID))
	_ = os.RemoveAll(runWork)
	if err := os.MkdirAll(runWork, 0o750); err != nil {
		return "", false, err
	}
	defer os.RemoveAll(runWork)

	archivePath := filepath.Join(runWork, archiveName)
	checksumPath := filepath.Join(runWork, checksumName)
	if err := m.downloadAsset(ctx, archiveURL, archivePath); err != nil {
		return "", false, err
	}
	if err := m.downloadAsset(ctx, checksumURL, checksumPath); err != nil {
		return "", false, err
	}
	if m.cfg.UpdateRequireSignature {
		signaturePath := filepath.Join(runWork, signatureName)
		if err := m.downloadAsset(ctx, signatureURL, signaturePath); err != nil {
			return "", false, err
		}
		if err := verifyChecksumSignature(checksumPath, signaturePath, m.cfg.UpdateSigningPublicKeys); err != nil {
			return "", false, err
		}
	}
	if err := verifyChecksumFile(checksumPath, archivePath, archiveName); err != nil {
		return "", false, err
	}

	extracted := filepath.Join(runWork, "extract")
	if err := extractTarGz(archivePath, extracted); err != nil {
		return "", false, err
	}
	payloadRoot, err := findPayloadRoot(extracted)
	if err != nil {
		return "", false, err
	}

	stageDir := filepath.Join(m.cfg.UpdateInstallDir, ".update-stage-"+sanitizePathToken(runID))
	if err := os.RemoveAll(stageDir); err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return "", false, err
	}
	defer os.RemoveAll(stageDir)

	if err := copyFile(filepath.Join(payloadRoot, "despatch"), filepath.Join(stageDir, "despatch"), 0o755); err != nil {
		return "", false, err
	}
	if err := copyFile(filepath.Join(payloadRoot, "despatch-pam-reset-helper"), filepath.Join(stageDir, "despatch-pam-reset-helper"), 0o755); err != nil {
		return "", false, err
	}
	if err := copyFile(filepath.Join(payloadRoot, "despatch-update-worker"), filepath.Join(stageDir, "despatch-update-worker"), 0o755); err != nil {
		return "", false, err
	}
	if err := copyDir(filepath.Join(payloadRoot, "web"), filepath.Join(stageDir, "web")); err != nil {
		return "", false, err
	}
	if err := copyDir(filepath.Join(payloadRoot, "migrations"), filepath.Join(stageDir, "migrations")); err != nil {
		return "", false, err
	}
	if err := copyDir(filepath.Join(payloadRoot, "deploy"), filepath.Join(stageDir, "deploy")); err != nil {
		return "", false, err
	}
	mailSecPayloadPath := filepath.Join(payloadRoot, "despatch-mailsec-service")
	mailSecPayloadPresent := false
	if _, err := os.Stat(mailSecPayloadPath); err == nil {
		if err := copyFile(mailSecPayloadPath, filepath.Join(stageDir, "despatch-mailsec-service"), 0o755); err != nil {
			return "", false, err
		}
		mailSecPayloadPresent = true
	} else if !os.IsNotExist(err) {
		return "", false, err
	}

	currentMailSec := filepath.Join(m.cfg.UpdateInstallDir, "despatch-mailsec-service")
	currentMailSecPresent := false
	if _, err := os.Stat(currentMailSec); err == nil {
		currentMailSecPresent = true
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	mailSecUnitKnownToSystemd := systemdUnitKnown(ctx, "despatch-mailsec.service")
	if m.cfg.MailSecEnabled && !mailSecPayloadPresent && !currentMailSecPresent {
		return "", false, fmt.Errorf("mailsec is enabled but despatch-mailsec-service is missing in both current install and release payload")
	}

	prevBin := filepath.Join(m.cfg.UpdateInstallDir, ".prev-despatch-"+sanitizePathToken(runID))
	prevPam := filepath.Join(m.cfg.UpdateInstallDir, ".prev-pam-reset-helper-"+sanitizePathToken(runID))
	prevWorker := filepath.Join(m.cfg.UpdateInstallDir, ".prev-update-worker-"+sanitizePathToken(runID))
	prevWeb := filepath.Join(m.cfg.UpdateInstallDir, ".prev-web-"+sanitizePathToken(runID))
	prevMig := filepath.Join(m.cfg.UpdateInstallDir, ".prev-migrations-"+sanitizePathToken(runID))
	prevDeploy := filepath.Join(m.cfg.UpdateInstallDir, ".prev-deploy-"+sanitizePathToken(runID))
	prevMailSec := filepath.Join(m.cfg.UpdateInstallDir, ".prev-mailsec-"+sanitizePathToken(runID))

	currentBin := filepath.Join(m.cfg.UpdateInstallDir, "despatch")
	currentPam := filepath.Join(m.cfg.UpdateInstallDir, "despatch-pam-reset-helper")
	currentWorker := filepath.Join(m.cfg.UpdateInstallDir, "despatch-update-worker")
	currentWeb := filepath.Join(m.cfg.UpdateInstallDir, "web")
	currentMig := filepath.Join(m.cfg.UpdateInstallDir, "migrations")
	currentDeploy := installDeployDir(m.cfg)
	currentMailSec = filepath.Join(m.cfg.UpdateInstallDir, "despatch-mailsec-service")
	stageBin := filepath.Join(stageDir, "despatch")
	stagePam := filepath.Join(stageDir, "despatch-pam-reset-helper")
	stageWorker := filepath.Join(stageDir, "despatch-update-worker")
	stageWeb := filepath.Join(stageDir, "web")
	stageMig := filepath.Join(stageDir, "migrations")
	stageDeploy := filepath.Join(stageDir, "deploy")
	stageMailSec := filepath.Join(stageDir, "despatch-mailsec-service")

	for _, p := range []string{prevBin, prevPam, prevWorker, prevWeb, prevMig, prevDeploy, prevMailSec} {
		_ = os.RemoveAll(p)
	}

	swapped := false
	pamSwapped := false
	workerSwapped := false
	deploySwapped := false
	mailSecSwapped := false
	rollback := func() error {
		_ = os.RemoveAll(currentWeb)
		_ = os.RemoveAll(currentMig)
		_ = os.RemoveAll(currentDeploy)
		_ = os.Remove(currentBin)
		if pamSwapped {
			_ = os.Remove(currentPam)
		}
		if workerSwapped {
			_ = os.Remove(currentWorker)
		}
		if mailSecSwapped {
			_ = os.Remove(currentMailSec)
		}
		if _, err := os.Stat(prevWeb); err == nil {
			if err := os.Rename(prevWeb, currentWeb); err != nil {
				return err
			}
		}
		if _, err := os.Stat(prevMig); err == nil {
			if err := os.Rename(prevMig, currentMig); err != nil {
				return err
			}
		}
		if _, err := os.Stat(prevBin); err == nil {
			if err := os.Rename(prevBin, currentBin); err != nil {
				return err
			}
		}
		if pamSwapped {
			if _, err := os.Stat(prevPam); err == nil {
				if err := os.Rename(prevPam, currentPam); err != nil {
					return err
				}
			}
		}
		if workerSwapped {
			if _, err := os.Stat(prevWorker); err == nil {
				if err := os.Rename(prevWorker, currentWorker); err != nil {
					return err
				}
			}
		}
		if deploySwapped {
			if _, err := os.Stat(prevDeploy); err == nil {
				if err := os.Rename(prevDeploy, currentDeploy); err != nil {
					return err
				}
			}
		}
		if mailSecSwapped {
			if _, err := os.Stat(prevMailSec); err == nil {
				if err := os.Rename(prevMailSec, currentMailSec); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := swapPath(currentBin, stageBin, prevBin); err != nil {
		return "", false, err
	}
	if err := swapPathOptional(currentPam, stagePam, prevPam); err != nil {
		_ = rollback()
		return "", false, err
	}
	pamSwapped = true
	if err := swapPathOptional(currentWorker, stageWorker, prevWorker); err != nil {
		_ = rollback()
		return "", false, err
	}
	workerSwapped = true
	if err := swapPath(currentWeb, stageWeb, prevWeb); err != nil {
		_ = rollback()
		return "", false, err
	}
	if err := swapPath(currentMig, stageMig, prevMig); err != nil {
		_ = rollback()
		return "", false, err
	}
	if err := swapPathOptional(currentDeploy, stageDeploy, prevDeploy); err != nil {
		_ = rollback()
		return "", false, err
	}
	deploySwapped = true
	if mailSecPayloadPresent {
		if err := swapPathOptional(currentMailSec, stageMailSec, prevMailSec); err != nil {
			_ = rollback()
			return "", false, err
		}
		mailSecSwapped = true
	}
	swapped = true

	mailSecUnitSource := filepath.Join(currentDeploy, "despatch-mailsec.service")
	mailSecUnitDst := filepath.Join(m.cfg.UpdateSystemdUnitDir, "despatch-mailsec.service")
	updaterServiceSource := filepath.Join(currentDeploy, "despatch-updater.service")
	updaterPathSource := filepath.Join(currentDeploy, "despatch-updater.path")
	updaterServiceDst := updaterServiceUnitPath(m.cfg)
	updaterPathDst := updaterPathUnitPath(m.cfg)
	if _, err := os.Stat(updaterServiceSource); err != nil {
		if rbErr := rollback(); rbErr != nil {
			return "", false, fmt.Errorf("updater service unit missing after deploy refresh: %v; rollback failed: %v", err, rbErr)
		}
		return "", true, fmt.Errorf("deploy payload is missing despatch-updater.service after refresh")
	}
	if _, err := os.Stat(updaterPathSource); err != nil {
		if rbErr := rollback(); rbErr != nil {
			return "", false, fmt.Errorf("updater path unit missing after deploy refresh: %v; rollback failed: %v", err, rbErr)
		}
		return "", true, fmt.Errorf("deploy payload is missing despatch-updater.path after refresh")
	}

	runtimeBeforeUnitRefresh := m.runtimeProbe(ctx, m.cfg)
	reloadRequired := false
	for _, unitCopy := range []struct {
		src         string
		dst         string
		description string
	}{
		{src: updaterServiceSource, dst: updaterServiceDst, description: "updater service"},
		{src: updaterPathSource, dst: updaterPathDst, description: "updater path"},
	} {
		if err := copyFile(unitCopy.src, unitCopy.dst, 0o644); err != nil {
			if isReadOnlyOrPermissionError(err) && runtimeBeforeUnitRefresh.Healthy() {
				continue
			}
			if rbErr := rollback(); rbErr != nil {
				return "", false, fmt.Errorf("%s unit install failed: %v; rollback failed: %v", unitCopy.description, err, rbErr)
			}
			return "", true, err
		}
		reloadRequired = true
	}
	if _, err := os.Stat(mailSecUnitSource); err == nil {
		if err := copyFile(mailSecUnitSource, mailSecUnitDst, 0o644); err != nil {
			if isReadOnlyOrPermissionError(err) && mailSecUnitKnownToSystemd {
				// Keep using the existing unit when systemd unit dir cannot be modified (e.g. read-only host rootfs).
			} else {
				if rbErr := rollback(); rbErr != nil {
					return "", false, fmt.Errorf("mailsec unit install failed: %v; rollback failed: %v", err, rbErr)
				}
				return "", true, err
			}
		}
		reloadRequired = true
	} else if err != nil && !os.IsNotExist(err) {
		if rbErr := rollback(); rbErr != nil {
			return "", false, fmt.Errorf("mailsec unit stat failed: %v; rollback failed: %v", err, rbErr)
		}
		return "", true, err
	}
	if reloadRequired {
		if err := runCmd(ctx, "systemctl", "daemon-reload"); err != nil {
			if rbErr := rollback(); rbErr != nil {
				return "", false, fmt.Errorf("systemd daemon-reload failed: %v; rollback failed: %v", err, rbErr)
			}
			return "", true, err
		}
	}
	_ = runCmd(ctx, "systemctl", "reset-failed", "despatch-updater.service", "despatch-updater.path")
	mailSecUnitNowPresent := false
	if _, err := os.Stat(mailSecUnitDst); err == nil {
		mailSecUnitNowPresent = true
	} else if !os.IsNotExist(err) {
		if rbErr := rollback(); rbErr != nil {
			return "", false, fmt.Errorf("mailsec unit stat failed: %v; rollback failed: %v", err, rbErr)
		}
		return "", true, err
	}
	if !mailSecUnitNowPresent {
		mailSecUnitNowPresent = systemdUnitKnown(ctx, "despatch-mailsec.service")
	}
	if err := runCmd(ctx, "systemctl", "enable", "--now", "despatch-updater.path"); err != nil {
		if rbErr := rollback(); rbErr != nil {
			return "", false, fmt.Errorf("updater path activation failed: %v; rollback failed: %v", err, rbErr)
		}
		return "", true, err
	}
	runtimeAfterUnitRefresh := m.runtimeProbe(ctx, m.cfg)
	if !runtimeAfterUnitRefresh.Healthy() {
		if rbErr := rollback(); rbErr != nil {
			return "", false, fmt.Errorf("updater runtime is unhealthy after unit refresh: %s; rollback failed: %v", runtimeAfterUnitRefresh.StaleQueueError(), rbErr)
		}
		return "", true, fmt.Errorf("updater runtime is unhealthy after unit refresh: %s", runtimeAfterUnitRefresh.StaleQueueError())
	}
	if m.cfg.MailSecEnabled && !mailSecUnitNowPresent {
		if rbErr := rollback(); rbErr != nil {
			return "", false, fmt.Errorf("mailsec unit missing after update; rollback failed: %v", rbErr)
		}
		return "", true, fmt.Errorf("mailsec is enabled but systemd unit despatch-mailsec.service is still missing after update")
	}
	if m.cfg.MailSecEnabled || mailSecPayloadPresent || mailSecUnitNowPresent {
		if err := runCmd(ctx, "systemctl", "enable", "--now", "despatch-mailsec"); err != nil {
			if rbErr := rollback(); rbErr != nil {
				return "", false, fmt.Errorf("mailsec enable failed: %v; rollback failed: %v", err, rbErr)
			}
			return "", true, err
		}
		if m.cfg.MailSecEnabled {
			if !waitForPath(m.cfg.MailSecSocket, 10*time.Second) {
				if rbErr := rollback(); rbErr != nil {
					return "", false, fmt.Errorf("mailsec socket missing after start; rollback failed: %v", rbErr)
				}
				return "", true, fmt.Errorf("mailsec is enabled but socket was not created at %s after start", strings.TrimSpace(m.cfg.MailSecSocket))
			}
		}
	}
	if _, err := migrateLegacyPasswordResetEnv(filepath.Join(m.cfg.UpdateInstallDir, ".env")); err != nil {
		if rbErr := rollback(); rbErr != nil {
			return "", false, fmt.Errorf("password reset env migration failed: %v; rollback failed: %v", err, rbErr)
		}
		return "", true, fmt.Errorf("password reset env migration failed: %v", err)
	}
	if err := runCmd(ctx, "systemctl", "restart", m.cfg.UpdateServiceName); err != nil {
		if rbErr := rollback(); rbErr != nil {
			return "", false, fmt.Errorf("service restart failed: %v; rollback failed: %v", err, rbErr)
		}
		_ = runCmd(context.Background(), "systemctl", "restart", m.cfg.UpdateServiceName)
		return "", true, err
	}
	if err := checkServiceHealth(ctx, m.cfg.ListenAddr); err != nil {
		if swapped {
			if rbErr := rollback(); rbErr != nil {
				return release.TagName, false, fmt.Errorf("health check failed: %v; rollback failed: %v", err, rbErr)
			}
			_ = runCmd(context.Background(), "systemctl", "restart", m.cfg.UpdateServiceName)
			return release.TagName, true, err
		}
		return release.TagName, false, err
	}

	backupDest := filepath.Join(backupsDir(m.cfg), time.Now().UTC().Format("20060102T150405")+"-"+sanitizePathToken(runID))
	if err := os.MkdirAll(backupDest, 0o750); err != nil {
		return release.TagName, false, err
	}
	_ = os.Rename(prevBin, filepath.Join(backupDest, "despatch"))
	_ = os.Rename(prevPam, filepath.Join(backupDest, "despatch-pam-reset-helper"))
	_ = os.Rename(prevWorker, filepath.Join(backupDest, "despatch-update-worker"))
	_ = os.Rename(prevMailSec, filepath.Join(backupDest, "despatch-mailsec-service"))
	_ = os.Rename(prevWeb, filepath.Join(backupDest, "web"))
	_ = os.Rename(prevMig, filepath.Join(backupDest, "migrations"))
	_ = os.Rename(prevDeploy, filepath.Join(backupDest, "deploy"))
	trimBackups(backupsDir(m.cfg), m.cfg.UpdateBackupKeep)

	return release.TagName, false, nil
}

func resolveArchiveAsset(rel githubRelease, arch string) (string, string, bool) {
	candidates := archiveAssetCandidates(arch)
	for _, wanted := range candidates {
		url, ok := findAssetURL(rel, wanted)
		if ok {
			return strings.TrimSpace(wanted), strings.TrimSpace(url), true
		}
	}
	return "", "", false
}

func archiveAssetCandidates(arch string) []string {
	normalized := strings.ToLower(strings.TrimSpace(arch))
	if normalized == "" {
		normalized = runtime.GOARCH
	}
	archAliases := []string{normalized}
	switch normalized {
	case "amd64", "x86_64", "x64":
		archAliases = append(archAliases, "amd64", "x86_64", "x64")
	case "arm64", "aarch64":
		archAliases = append(archAliases, "arm64", "aarch64")
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(archAliases)*4)
	add := func(name string) {
		n := strings.TrimSpace(strings.ToLower(name))
		if n == "" {
			return
		}
		if _, ok := seen[n]; ok {
			return
		}
		seen[n] = struct{}{}
		out = append(out, name)
	}
	for _, alias := range archAliases {
		a := strings.TrimSpace(strings.ToLower(alias))
		if a == "" {
			continue
		}
		add(fmt.Sprintf("despatch-linux-%s.tar.gz", a))
		add(fmt.Sprintf("despatch-linux-%s.tgz", a))
		add(fmt.Sprintf("despatch_%s_linux.tar.gz", a))
		add(fmt.Sprintf("despatch_%s_linux.tgz", a))
	}
	return out
}

func releaseAssetNames(rel githubRelease) []string {
	names := make([]string, 0, len(rel.Assets))
	for _, asset := range rel.Assets {
		name := strings.TrimSpace(asset.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return []string{"<none>"}
	}
	sort.Strings(names)
	return names
}

func (m *Manager) resolveRelease(ctx context.Context, targetVersion string) (githubRelease, error) {
	if targetVersion != "" {
		return m.gh.releaseByTag(ctx, targetVersion)
	}
	rel, _, _, err := m.gh.latestPreferredReleaseRaw(ctx, "")
	return rel, err
}

func (m *Manager) downloadAsset(ctx context.Context, rawURL, destPath string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return fmt.Errorf("refusing non-https asset url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "despatch-updater/1")
	if token := strings.TrimSpace(m.cfg.UpdateGitHubToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: time.Duration(m.cfg.UpdateHTTPTimeoutSec) * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("asset download failed with status %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return err
	}
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func verifyChecksumFile(checksumPath, archivePath, archiveName string) error {
	raw, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}
	expected := ""
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		if fields[1] == archiveName || strings.TrimPrefix(fields[1], "*") == archiveName {
			expected = fields[0]
			break
		}
	}
	if expected == "" {
		return fmt.Errorf("checksum for %s not found", archiveName)
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(strings.TrimSpace(expected), got) {
		return fmt.Errorf("checksum mismatch for %s", archiveName)
	}
	return nil
}

func verifyChecksumSignature(checksumPath, signaturePath string, publicKeys []string) error {
	if len(publicKeys) == 0 {
		return fmt.Errorf("no update signing keys configured")
	}
	checksumBytes, err := os.ReadFile(checksumPath)
	if err != nil {
		return err
	}
	signatureRaw, err := os.ReadFile(signaturePath)
	if err != nil {
		return err
	}
	signature, err := decodeSignaturePayload(signatureRaw)
	if err != nil {
		return err
	}
	for _, key := range publicKeys {
		pub, err := decodePublicKey(key)
		if err != nil {
			return err
		}
		if ed25519.Verify(pub, checksumBytes, signature) {
			return nil
		}
	}
	return fmt.Errorf("signature verification failed for checksums file")
}

func decodePublicKey(raw string) (ed25519.PublicKey, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return nil, fmt.Errorf("empty update signing key")
	}
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(key)
		if err != nil {
			return nil, fmt.Errorf("invalid update signing key encoding")
		}
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid update signing key size")
	}
	return ed25519.PublicKey(decoded), nil
}

func decodeSignaturePayload(raw []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("empty update signature")
	}
	decoded, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(trimmed)
	}
	if err == nil {
		if len(decoded) != ed25519.SignatureSize {
			return nil, fmt.Errorf("invalid update signature size")
		}
		return decoded, nil
	}
	if len(raw) == ed25519.SignatureSize {
		return raw, nil
	}
	return nil, fmt.Errorf("invalid update signature encoding")
}

func extractTarGz(src, dest string) error {
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return err
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(strings.TrimPrefix(hdr.Name, "/"))
		if name == "." || strings.HasPrefix(name, "..") {
			return fmt.Errorf("invalid archive path: %q", hdr.Name)
		}
		target := filepath.Join(dest, name)
		if !strings.HasPrefix(target, dest+string(os.PathSeparator)) && target != dest {
			return fmt.Errorf("invalid archive path: %q", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			_ = out.Close()
		}
	}
	return nil
}

func findPayloadRoot(extractDir string) (string, error) {
	required := []string{"despatch", "despatch-pam-reset-helper", "despatch-update-worker", "web", "migrations", "deploy"}
	if hasRequiredPaths(extractDir, required) {
		return extractDir, nil
	}
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return "", err
	}
	if len(entries) == 1 && entries[0].IsDir() {
		root := filepath.Join(extractDir, entries[0].Name())
		if hasRequiredPaths(root, required) {
			return root, nil
		}
	}
	return "", fmt.Errorf("release payload missing required files (despatch, despatch-pam-reset-helper, despatch-update-worker, web, migrations, deploy)")
}

func hasRequiredPaths(root string, required []string) bool {
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return false
		}
	}
	return true
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode())
	})
}

func swapPath(current, staged, previous string) error {
	if err := os.Rename(current, previous); err != nil {
		return err
	}
	if err := os.Rename(staged, current); err != nil {
		_ = os.Rename(previous, current)
		return err
	}
	return nil
}

func swapPathOptional(current, staged, previous string) error {
	currentExists := true
	if _, err := os.Stat(current); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		currentExists = false
	}
	if currentExists {
		if err := os.Rename(current, previous); err != nil {
			return err
		}
	}
	if err := os.Rename(staged, current); err != nil {
		if currentExists {
			_ = os.Rename(previous, current)
		}
		return err
	}
	return nil
}

func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %v (%s)", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func systemdUnitKnown(ctx context.Context, unitName string) bool {
	unit := strings.TrimSpace(unitName)
	if unit == "" {
		return false
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, "systemctl", "show", "--property=LoadState", "--value", unit)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return isSystemdLoadStateKnown(string(out))
}

func isSystemdLoadStateKnown(raw string) bool {
	state := strings.ToLower(strings.TrimSpace(raw))
	switch state {
	case "", "not-found", "error", "bad-setting":
		return false
	default:
		return true
	}
}

func isReadOnlyOrPermissionError(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EROFS)
}

func checkServiceHealth(ctx context.Context, listenAddr string) error {
	base := localBaseURL(listenAddr)
	client := &http.Client{Timeout: 5 * time.Second}
	endpoints := []string{"/health/live", "/api/v1/setup/status"}
	deadline := time.Now().Add(40 * time.Second)
	for {
		allOK := true
		for _, ep := range endpoints {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+ep, nil)
			resp, err := client.Do(req)
			if err != nil {
				allOK = false
				break
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				allOK = false
				break
			}
		}
		if allOK {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("health checks failed after restart")
		}
		time.Sleep(1200 * time.Millisecond)
	}
}

func waitForPath(path string, timeout time.Duration) bool {
	target := strings.TrimSpace(path)
	if target == "" {
		return false
	}
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(target); err == nil {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func localBaseURL(listenAddr string) string {
	addr := strings.TrimSpace(listenAddr)
	if addr == "" {
		return "http://127.0.0.1:8080"
	}
	if strings.HasPrefix(addr, ":") {
		return "http://127.0.0.1" + addr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://127.0.0.1:8080"
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func trimBackups(base string, keep int) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	type item struct {
		name string
		time time.Time
	}
	all := make([]item, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		all = append(all, item{name: e.Name(), time: info.ModTime()})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].time.After(all[j].time) })
	for i := keep; i < len(all); i++ {
		_ = os.RemoveAll(filepath.Join(base, all[i].name))
	}
}

func migrateLegacyPasswordResetEnv(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	values := envFileValues(lines)
	if !shouldMigrateLegacyPasswordResetEnv(values) {
		return false, nil
	}
	baseDomain := preferredPublicResetHost(values)
	resetFrom := envValueTrimmed(values, "PASSWORD_RESET_FROM")
	targetFrom := "no-reply@" + derivePasswordResetSenderDomain(baseDomain)
	if shouldRewritePasswordResetFrom(resetFrom, values) {
		envFileSet(&lines, "PASSWORD_RESET_FROM", targetFrom)
	}
	if shouldRewritePasswordResetBaseURL(envValueTrimmed(values, "PASSWORD_RESET_BASE_URL")) {
		envFileSet(&lines, "PASSWORD_RESET_BASE_URL", derivePasswordResetBaseURL(values))
	}
	envFileSet(&lines, "SMTP_PORT", "25")
	envFileSet(&lines, "SMTP_STARTTLS", "false")
	envFileSet(&lines, "SMTP_TLS", "false")
	if strings.EqualFold(envValueTrimmed(values, "DOVECOT_AUTH_MODE"), "pam") &&
		!strings.EqualFold(envValueTrimmed(values, "PASSWORD_RESET_SENDER"), "log") {
		envFileSet(&lines, "PASSWORD_RESET_EXTERNAL_SENDER_READY", "true")
	}
	tmp := path + ".tmp-reset-migration"
	output := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(tmp, []byte(output), info.Mode().Perm()); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return false, err
	}
	return true, nil
}

func envFileValues(lines []string) map[string]string {
	values := make(map[string]string, len(lines))
	for _, line := range lines {
		if idx := strings.IndexByte(line, '='); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			if key == "" || strings.HasPrefix(key, "#") {
				continue
			}
			values[key] = strings.TrimSpace(line[idx+1:])
		}
	}
	return values
}

func envFileSet(lines *[]string, key, value string) {
	prefix := key + "="
	for i, line := range *lines {
		if strings.HasPrefix(line, prefix) {
			(*lines)[i] = prefix + value
			return
		}
	}
	*lines = append(*lines, prefix+value)
}

func envValueTrimmed(values map[string]string, key string) string {
	return strings.TrimSpace(values[key])
}

func shouldMigrateLegacyPasswordResetEnv(values map[string]string) bool {
	if strings.EqualFold(envValueTrimmed(values, "PASSWORD_RESET_SENDER"), "log") {
		return false
	}
	if !isLoopbackResetSMTPHost(envValueTrimmed(values, "SMTP_HOST")) {
		return false
	}
	if envValueTrimmed(values, "SMTP_PORT") != "587" {
		return false
	}
	if !envFlagTrue(values, "SMTP_STARTTLS") || envFlagTrue(values, "SMTP_TLS") {
		return false
	}
	if envValueTrimmed(values, "PASSWORD_RESET_SMTP_USER") != "" || envValueTrimmed(values, "SMTP_USER") != "" {
		return false
	}
	return true
}

func envFlagTrue(values map[string]string, key string) bool {
	switch strings.ToLower(strings.TrimSpace(values[key])) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func isLoopbackResetSMTPHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "127.0.0.1", "localhost", "::1", "[::1]":
		return true
	default:
		return false
	}
}

func preferredPublicResetHost(values map[string]string) string {
	if strings.EqualFold(envValueTrimmed(values, "DEPLOY_MODE"), "proxy") {
		if host := envValueTrimmed(values, "PROXY_SERVER_NAME"); host != "" {
			return host
		}
	}
	return envValueTrimmed(values, "BASE_DOMAIN")
}

func derivePasswordResetSenderDomain(host string) string {
	domain := strings.ToLower(strings.TrimSpace(host))
	if strings.HasPrefix(domain, "mail.") && len(domain) > len("mail.") {
		return domain[len("mail."):]
	}
	if domain == "" {
		return "example.com"
	}
	return domain
}

func shouldRewritePasswordResetFrom(current string, values map[string]string) bool {
	current = strings.ToLower(strings.TrimSpace(current))
	if current == "" || strings.HasSuffix(current, "@example.com") {
		return true
	}
	base := strings.ToLower(strings.TrimSpace(envValueTrimmed(values, "BASE_DOMAIN")))
	publicHost := strings.ToLower(strings.TrimSpace(preferredPublicResetHost(values)))
	return current == "no-reply@"+base || current == "no-reply@"+publicHost
}

func shouldRewritePasswordResetBaseURL(current string) bool {
	current = strings.ToLower(strings.TrimSpace(current))
	if current == "" {
		return true
	}
	return strings.Contains(current, "127.0.0.1") ||
		strings.Contains(current, "localhost") ||
		strings.Contains(current, "example.com")
}

func derivePasswordResetBaseURL(values map[string]string) string {
	if strings.EqualFold(envValueTrimmed(values, "DEPLOY_MODE"), "proxy") {
		host := envValueTrimmed(values, "PROXY_SERVER_NAME")
		if host == "" {
			host = envValueTrimmed(values, "BASE_DOMAIN")
		}
		if host == "" {
			host = "127.0.0.1"
		}
		if envFlagTrue(values, "PROXY_TLS") {
			return "https://" + host
		}
		return "http://" + host
	}
	listen := envValueTrimmed(values, "LISTEN_ADDR")
	host := ""
	port := ""
	switch {
	case strings.HasPrefix(listen, ":"):
		host = envValueTrimmed(values, "BASE_DOMAIN")
		port = strings.TrimPrefix(listen, ":")
	case strings.Contains(listen, ":"):
		host = strings.Trim(strings.TrimSpace(listen[:strings.LastIndex(listen, ":")]), "[]")
		port = strings.TrimSpace(listen[strings.LastIndex(listen, ":")+1:])
	default:
		host = envValueTrimmed(values, "BASE_DOMAIN")
		port = "8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" || isLoopbackResetSMTPHost(host) {
		host = envValueTrimmed(values, "BASE_DOMAIN")
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "8080"
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func sanitizePathToken(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range v {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}

func ensureDespatchReadable(path string) error {
	u, err := user.Lookup("despatch")
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return err
	}
	if err := os.Chown(path, -1, gid); err != nil {
		return err
	}
	// Keep file private to root/despatch while allowing app user reads.
	return os.Chmod(path, 0o640)
}
