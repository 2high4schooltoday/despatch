package mail

import "testing"

func TestNormalizeThreadSubject(t *testing.T) {
	got := NormalizeThreadSubject("  Re: FWD: fw:  Status Update  ")
	want := "status update"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDeriveThreadIDStableAndMailboxScoped(t *testing.T) {
	a := DeriveThreadID("INBOX", "Re: Weekly Sync", "alice@example.com")
	b := DeriveThreadID("INBOX", "weekly sync", "alice@example.com")
	if a != b {
		t.Fatalf("expected stable thread id, got %q and %q", a, b)
	}

	otherMailbox := DeriveThreadID("Archive", "weekly sync", "alice@example.com")
	if otherMailbox == a {
		t.Fatalf("expected different thread id across mailboxes, got same %q", a)
	}
}

func TestBuildPreviewFromBodySample(t *testing.T) {
	sample := "<p>Hello&nbsp;&nbsp;team</p>\n\n This is a test   message."
	got := BuildPreviewFromBodySample(sample, 20)
	if got != "Hello team This is a" {
		t.Fatalf("unexpected preview: %q", got)
	}
}
