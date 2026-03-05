package update

import (
	"context"
	"crypto/ed25519"
	crand "crypto/rand"
	"encoding/base64"
	"errors"
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
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "despatch-updater.path"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write unit file: %v", err)
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
	if _, err := mgr.QueueApply(context.Background(), st, "admin@example.com", "v1.2.4", "req-2"); err == nil {
		t.Fatalf("expected in-progress protection on second request")
	}
}

func TestQueueApplyIgnoresUnreadableStatusFile(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "despatch-updater.path"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write unit file: %v", err)
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
	mgr := NewManager(cfg)
	if err := os.MkdirAll(statusDir(cfg), 0o755); err != nil {
		t.Fatalf("mkdir status dir: %v", err)
	}
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
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "despatch-updater.path"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write updater marker: %v", err)
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
}

func TestStatusConfiguredWhenUpdaterReady(t *testing.T) {
	st := newUpdateTestStore(t)
	base := t.TempDir()
	unitDir := filepath.Join(base, "units")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		t.Fatalf("mkdir unit dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(unitDir, "despatch-updater.path"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write updater marker: %v", err)
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
	if !status.Configured {
		t.Fatalf("expected configured=true when updater marker and writable paths are present")
	}
	if status.ConfigDiagnostic != nil {
		t.Fatalf("expected no config diagnostic when updater is configured, got %#v", status.ConfigDiagnostic)
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
