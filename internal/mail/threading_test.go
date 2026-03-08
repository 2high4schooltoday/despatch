package mail

import (
	"strings"
	"testing"
)

func TestNormalizeThreadSubject(t *testing.T) {
	got := NormalizeThreadSubject("  Re: FWD: fw:  Status Update  ")
	want := "status update"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDeriveThreadIDStableAndConversationScoped(t *testing.T) {
	a := DeriveThreadID("INBOX", "Re: Weekly Sync", "alice@example.com")
	b := DeriveThreadID("INBOX", "weekly sync", "alice@example.com")
	if a != b {
		t.Fatalf("expected stable thread id, got %q and %q", a, b)
	}

	otherMailbox := DeriveThreadID("Archive", "weekly sync", "alice@example.com")
	if otherMailbox != a {
		t.Fatalf("expected same thread id across mailboxes, got %q and %q", a, otherMailbox)
	}
}

func TestBuildPreviewFromBodySample(t *testing.T) {
	sample := "<p>Hello&nbsp;&nbsp;team</p>\n\n This is a test   message."
	got := BuildPreviewFromBodySample(sample, 20)
	if got != "Hello team This is a" {
		t.Fatalf("unexpected preview: %q", got)
	}
}

func TestBuildPreviewFromMIMERawSamplePrefersPlainPart(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: Alice <alice@example.com>",
		"To: Bob <bob@example.com>",
		"Subject: Preview Test",
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=\"alt\"",
		"",
		"--alt",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Plain text preview wins.",
		"--alt",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>HTML fallback</p>",
		"--alt--",
	}, "\r\n"))

	got := BuildPreviewFromMIMERawSample(raw, 120)
	if got != "Plain text preview wins." {
		t.Fatalf("unexpected mime preview: %q", got)
	}
}

func TestBuildPreviewFromMIMERawSampleFallsBackToHTML(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: Alice <alice@example.com>",
		"To: Bob <bob@example.com>",
		"Subject: HTML only",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p><strong>Hello</strong> from HTML.</p>",
	}, "\r\n"))

	got := BuildPreviewFromMIMERawSample(raw, 120)
	if got != "Hello from HTML." {
		t.Fatalf("unexpected html fallback preview: %q", got)
	}
}

func TestBuildPreviewFromMIMERawSampleFallbackOnLongHeaders(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"X-Noise: " + strings.Repeat("abcdef", 80),
		"From: Alice <alice@example.com>",
		"To: Bob <bob@example.com>",
		"Subject: Header heavy",
		"",
		"Body still has useful preview text after long headers.",
	}, "\r\n"))

	got := BuildPreviewFromMIMERawSample(raw, 120)
	if got == "" || strings.Contains(strings.ToLower(got), "x-noise") {
		t.Fatalf("expected body-focused preview, got %q", got)
	}
}

func TestMailboxRoleResolutionSupportsAttributesAndCommonNames(t *testing.T) {
	if got := MailboxRole("Custom Sent Folder", []string{"\\Sent"}); got != "sent" {
		t.Fatalf("expected attribute-based sent role, got %q", got)
	}
	if got := MailboxRole("Deleted Messages", nil); got != "trash" {
		t.Fatalf("expected Deleted Messages trash role, got %q", got)
	}
	if got := ResolveMailboxByRole([]Mailbox{
		{Name: "INBOX", Role: "inbox"},
		{Name: "Sent Messages", Role: "sent"},
		{Name: "Deleted Messages", Role: "trash"},
	}, "sent"); got != "Sent Messages" {
		t.Fatalf("expected Sent Messages mailbox resolution, got %q", got)
	}
}
