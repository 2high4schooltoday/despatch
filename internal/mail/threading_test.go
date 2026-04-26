package mail

import (
	"encoding/base64"
	"encoding/hex"
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

func TestNormalizeThreadSubjectSupportsListTagsAndLocalizedPrefixes(t *testing.T) {
	got := NormalizeThreadSubject(" [Team] AW: Re[2]: 回复: Status Update (fwd) ")
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

func TestNormalizeMessageIDHeaderExtractsCanonicalToken(t *testing.T) {
	got := NormalizeMessageIDHeader(`reply to root <Root.ID@example.COM> (comment)`)
	if got != "root.id@example.com" {
		t.Fatalf("expected canonical message id, got %q", got)
	}
}

func TestDeriveIndexedThreadIDWithConversationIndexRootMatchesReplies(t *testing.T) {
	rootBytes := make([]byte, 22)
	for i := range rootBytes {
		rootBytes[i] = byte(i + 1)
	}
	replyBytes := append(append([]byte{}, rootBytes...), []byte{23, 24, 25, 26, 27}...)
	rootIndex := base64.StdEncoding.EncodeToString(rootBytes)
	replyIndex := base64.StdEncoding.EncodeToString(replyBytes)

	rootThreadID := DeriveIndexedThreadIDWithHints(
		"<root@example.com>",
		"",
		nil,
		"Topic",
		"alice@example.com",
		[]string{"alice@example.com", "user@example.com"},
		"Topic",
		rootIndex,
		"",
	)
	replyThreadID := DeriveIndexedThreadIDWithHints(
		"<reply@example.com>",
		"",
		nil,
		"Re: Topic",
		"user@example.com",
		[]string{"user@example.com", "alice@example.com"},
		"Topic",
		replyIndex,
		"",
	)
	if rootThreadID == "" || replyThreadID == "" || rootThreadID != replyThreadID {
		t.Fatalf("expected matching conversation-index thread ids, got root=%q reply=%q", rootThreadID, replyThreadID)
	}

	expectedRoot := hex.EncodeToString(rootBytes)
	if got := NormalizeConversationIndexRoot(rootIndex); got != expectedRoot {
		t.Fatalf("expected normalized root %q, got %q", expectedRoot, got)
	}
	if got := NormalizeConversationIndexRoot(replyIndex); got != expectedRoot {
		t.Fatalf("expected reply to keep same normalized root %q, got %q", expectedRoot, got)
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

func TestBuildPreviewFromBodySampleStripsCSSAndMachineNoise(t *testing.T) {
	sample := `<style>table {border-collapse:collapse} td {font-family:Arial}</style>
UEsDBAoAAAAIADZJaVy+kxu+8AEAAAMFAAALAAgCW0NvbnRlbnRfVHlwZXNdLnhtbCCiBAIooAAC
<p>Invoice attached. Please review this week.</p>`
	got := BuildPreviewFromBodySample(sample, 120)
	if strings.Contains(strings.ToLower(got), "border-collapse") || strings.Contains(got, "UEsDBA") {
		t.Fatalf("expected css/base64 noise removed, got %q", got)
	}
	if !strings.Contains(got, "Invoice attached") {
		t.Fatalf("expected human preview text preserved, got %q", got)
	}
}

func TestBuildPreviewFromBodySampleTruncatesInlineCSSTail(t *testing.T) {
	sample := `Your ChatGPT code is 680860 @font-face "Söhne"; src: url(data:font/woff2;base64,AAAA)`
	got := BuildPreviewFromBodySample(sample, 120)
	if got != "Your ChatGPT code is 680860" {
		t.Fatalf("expected inline css tail removed, got %q", got)
	}
}

func TestBuildPreviewFromBodySampleDropsTrackingLinks(t *testing.T) {
	sample := `Read this update: https://example.com/click?utm_source=test&utm_medium=email&token=abcdef1234567890abcdef1234567890 Thanks.`
	got := BuildPreviewFromBodySample(sample, 120)
	if strings.Contains(strings.ToLower(got), "utm_source") || strings.Contains(strings.ToLower(got), "token=") {
		t.Fatalf("expected tracking link noise removed, got %q", got)
	}
	if !strings.Contains(got, "Read this update:") {
		t.Fatalf("expected readable text kept, got %q", got)
	}
}

func TestBestAvailablePreviewFallsBackFromNoisySnippet(t *testing.T) {
	raw := strings.Join([]string{
		"From: Alice <alice@example.com>",
		"To: Bob <bob@example.com>",
		"Subject: Invoice",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<html><body><p>Invoice attached. Please review this week.</p></body></html>",
	}, "\r\n")

	got := BestAvailablePreview(
		`table {border-collapse:collapse} td {font-family:Arial}`,
		"",
		"",
		raw,
		120,
	)
	if got != "Invoice attached. Please review this week." {
		t.Fatalf("expected raw-source fallback preview, got %q", got)
	}
}

func TestBestAvailablePreviewKeepsCleanedSnippet(t *testing.T) {
	got := BestAvailablePreview("<p>Hello <strong>team</strong></p>", "", "", "", 120)
	if got != "Hello team" {
		t.Fatalf("expected cleaned snippet to be preserved, got %q", got)
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

func TestBuildReplyHeadersAppendsParentMessageID(t *testing.T) {
	inReplyTo, references := BuildReplyHeaders(
		"<parent@example.com>",
		"<older@example.com>",
		[]string{"<root@example.com>"},
	)
	if inReplyTo != "parent@example.com" {
		t.Fatalf("expected normalized in-reply-to, got %q", inReplyTo)
	}
	if len(references) != 2 || references[0] != "root@example.com" || references[1] != "parent@example.com" {
		t.Fatalf("unexpected reply references: %#v", references)
	}
}
