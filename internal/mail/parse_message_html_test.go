package mail

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseMessageMultipartAlternativeCapturesPlainAndHTML(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: Alice <alice@example.com>",
		"To: Bob <bob@example.com>",
		"Subject: Alternative",
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=alt-1",
		"",
		"--alt-1",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Plain body line.",
		"--alt-1",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<html><body><p>Hello <strong>HTML</strong></p></body></html>",
		"--alt-1--",
		"",
	}, "\r\n"))

	msg, err := parseMessage(raw, "msg-1", "INBOX", 101)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if got := strings.TrimSpace(msg.Body); got != "Plain body line." {
		t.Fatalf("expected plain body, got %q", got)
	}
	if !strings.Contains(msg.BodyHTML, "<strong>HTML</strong>") {
		t.Fatalf("expected html body to be captured, got %q", msg.BodyHTML)
	}
}

func TestParseMessageHTMLOnlyFallsBackToPlainText(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: Alice <alice@example.com>",
		"To: Bob <bob@example.com>",
		"Subject: HTML only",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<html><body><h1>Weekly update</h1><p>Status green.</p></body></html>",
		"",
	}, "\r\n"))

	msg, err := parseMessage(raw, "msg-2", "INBOX", 102)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if strings.TrimSpace(msg.BodyHTML) == "" {
		t.Fatalf("expected html body")
	}
	plain := strings.TrimSpace(msg.Body)
	if plain == "" {
		t.Fatalf("expected plain text fallback from html")
	}
	if !strings.Contains(strings.ToLower(plain), "weekly update") {
		t.Fatalf("expected fallback plain text to include heading, got %q", plain)
	}
}

func TestPlainTextFromHTMLStripsStyleAndScriptContents(t *testing.T) {
	rawHTML := `<html><head><style>@font-face{font-family:"Söhne";src:url(data:font/woff2;base64,AAAA)}body{font-family:"Söhne"}</style><script>console.log("nope")</script></head><body><p>Your ChatGPT code is 680860</p></body></html>`
	got := strings.TrimSpace(PlainTextFromHTML(rawHTML))
	if got != "Your ChatGPT code is 680860" {
		t.Fatalf("expected style/script content removed from html plaintext, got %q", got)
	}
}

func TestParseMessageIncludesInlineCIDAttachmentAndExtractParity(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: Alice <alice@example.com>",
		"To: Bob <bob@example.com>",
		"Subject: Related",
		"MIME-Version: 1.0",
		"Content-Type: multipart/related; boundary=rel-1",
		"",
		"--rel-1",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<html><body><img src=\"cid:Logo.CID\"></body></html>",
		"--rel-1",
		"Content-Type: image/png",
		"Content-Transfer-Encoding: base64",
		"Content-Disposition: inline",
		"Content-ID: <Logo.CID>",
		"",
		"AQIDBA==",
		"--rel-1--",
		"",
	}, "\r\n"))

	msgID := "msg-3"
	msg, err := parseMessage(raw, msgID, "INBOX", 103)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if len(msg.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(msg.Attachments))
	}
	meta := msg.Attachments[0]
	if !meta.Inline {
		t.Fatalf("expected inline attachment metadata")
	}
	if meta.ContentID != "Logo.CID" {
		t.Fatalf("expected normalized content-id without angle brackets, got %q", meta.ContentID)
	}
	if got := strings.ToLower(meta.ContentType); got != "image/png" {
		t.Fatalf("expected image/png content type, got %q", got)
	}
	if strings.TrimSpace(meta.Filename) == "" {
		t.Fatalf("expected deterministic fallback filename for inline attachment")
	}

	_, part, err := DecodeAttachmentID(meta.ID)
	if err != nil {
		t.Fatalf("decode attachment id: %v", err)
	}
	extractedMeta, data, err := extractAttachment(raw, msgID, part)
	if err != nil {
		t.Fatalf("extractAttachment: %v", err)
	}
	if extractedMeta.ContentID != meta.ContentID {
		t.Fatalf("expected content-id parity between parse and extract, got %q vs %q", extractedMeta.ContentID, meta.ContentID)
	}
	if !bytes.Equal(data, []byte{1, 2, 3, 4}) {
		t.Fatalf("unexpected extracted payload: %v", data)
	}
}

func TestParseMessageDecodesRFC2047DisplayNamesAndSubject(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"From: =?utf-8?q?=D0=9C=D0=B5=D0=B4=D0=A2=D0=BE=D1=87=D0=BA=D0=B0?= <info@medtochka.ru>",
		"To: =?utf-8?b?0JrQvtC80LDQvdC00LAg0JjQstCw0L3QvtCy0LA=?= <user@example.com>",
		"Subject: =?utf-8?b?0KLQtdGB0YLQvtCy0L7QtSDQv9C40YHRjNC80L4=?=",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		"Hello.",
		"",
	}, "\r\n"))

	msg, err := parseMessage(raw, "msg-rfc2047", "INBOX", 104)
	if err != nil {
		t.Fatalf("parseMessage: %v", err)
	}
	if got := msg.From; got != "МедТочка <info@medtochka.ru>" {
		t.Fatalf("expected decoded from, got %q", got)
	}
	if len(msg.To) != 1 || msg.To[0] != "Команда Иванова <user@example.com>" {
		t.Fatalf("expected decoded to, got %#v", msg.To)
	}
	if got := msg.Subject; got != "Тестовое письмо" {
		t.Fatalf("expected decoded subject, got %q", got)
	}
}
