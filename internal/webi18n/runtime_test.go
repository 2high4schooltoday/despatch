package webi18n

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadRussianBundle(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	localesDir := filepath.Join(filepath.Dir(filename), "..", "..", "web", "locales")
	runtimeI18n, err := Load(localesDir, "despatch")
	if err != nil {
		if strings.Contains(err.Error(), "unable to locate the Launchpad Rosetta native library") {
			t.Skip(err.Error())
		}
		t.Fatalf("Load failed: %v", err)
	}
	defer runtimeI18n.Close()

	bundle, ok := runtimeI18n.BundleFor("ru-RU")
	if !ok {
		t.Fatal("expected Russian bundle")
	}
	if bundle.Locale != "ru" {
		t.Fatalf("unexpected locale %q", bundle.Locale)
	}
	if got := bundle.Entries["theme_machine"].Value; got != "Тема: Машинная" {
		t.Fatalf("unexpected theme_machine value %q", got)
	}
	recipientCount := bundle.Entries["recipient_count"]
	if recipientCount.Kind != "plural" {
		t.Fatalf("recipient_count kind = %q", recipientCount.Kind)
	}
	if got := recipientCount.Forms["many"]; got != "{count} получателей" {
		t.Fatalf("unexpected recipient_count many form %q", got)
	}
}

func TestLoadRussianBundleFromPackagedNativeLibraryPath(t *testing.T) {
	sourceLibrary := findExistingNativeLibrary(t)
	if sourceLibrary == "" {
		t.Skip("Launchpad Rosetta native library is not built locally")
	}

	tempRoot := t.TempDir()
	localesDir := filepath.Join(tempRoot, "web", "locales")
	if err := os.MkdirAll(localesDir, 0o755); err != nil {
		t.Fatalf("mkdir locales dir: %v", err)
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	sourceLocalePath := filepath.Join(filepath.Dir(filename), "..", "..", "web", "locales", "ru.lpr")
	localeBytes, err := os.ReadFile(sourceLocalePath)
	if err != nil {
		t.Fatalf("read source locale: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localesDir, "ru.lpr"), localeBytes, 0o644); err != nil {
		t.Fatalf("write temp locale: %v", err)
	}

	libraryDest := filepath.Join(tempRoot, filepath.Base(sourceLibrary))
	libraryBytes, err := os.ReadFile(sourceLibrary)
	if err != nil {
		t.Fatalf("read source library: %v", err)
	}
	if err := os.WriteFile(libraryDest, libraryBytes, 0o755); err != nil {
		t.Fatalf("write temp library: %v", err)
	}

	previous := os.Getenv("LP_I18N_LIBRARY_PATH")
	if err := os.Unsetenv("LP_I18N_LIBRARY_PATH"); err != nil {
		t.Fatalf("unset LP_I18N_LIBRARY_PATH: %v", err)
	}
	defer func() {
		if previous == "" {
			_ = os.Unsetenv("LP_I18N_LIBRARY_PATH")
			return
		}
		_ = os.Setenv("LP_I18N_LIBRARY_PATH", previous)
	}()

	runtimeI18n, err := Load(localesDir, "despatch")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer runtimeI18n.Close()

	if got := os.Getenv("LP_I18N_LIBRARY_PATH"); got != libraryDest {
		t.Fatalf("expected LP_I18N_LIBRARY_PATH=%q, got %q", libraryDest, got)
	}
	if _, ok := runtimeI18n.BundleFor("ru"); !ok {
		t.Fatal("expected Russian bundle from packaged native library path")
	}
}

func findExistingNativeLibrary(t *testing.T) string {
	t.Helper()
	names := nativeLibraryNames()
	if len(names) == 0 {
		return ""
	}
	if explicit := strings.TrimSpace(os.Getenv("LP_I18N_LIBRARY_PATH")); explicit != "" {
		info, err := os.Stat(explicit)
		if err == nil && !info.IsDir() {
			return explicit
		}
		if err == nil && info.IsDir() {
			for _, name := range names {
				candidate := filepath.Join(explicit, name)
				if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
					return candidate
				}
			}
		}
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	searchRoots := []string{
		filepath.Join(repoRoot, "third_party", "launchpad-rosetta", "target", "debug"),
		filepath.Join(repoRoot, "third_party", "launchpad-rosetta", "target", "release"),
	}
	for _, searchRoot := range searchRoots {
		for _, name := range names {
			candidate := filepath.Join(searchRoot, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}
