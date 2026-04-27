package webi18n

import (
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
