package mail

import (
	"context"
	"strings"
	"testing"
)

func TestBuildRFC822IncludesCcAndOmitsBccHeader(t *testing.T) {
	raw, err := buildRFC822(SendRequest{
		From:    "sender@example.com",
		To:      []string{"to@example.com"},
		CC:      []string{"copy@example.com"},
		BCC:     []string{"hidden@example.com"},
		Subject: "Subject",
		Body:    "Body",
	})
	if err != nil {
		t.Fatalf("buildRFC822: %v", err)
	}
	msg := string(raw)
	if !strings.Contains(msg, "To: to@example.com") {
		t.Fatalf("expected To header, got=%q", msg)
	}
	if !strings.Contains(msg, "Cc: copy@example.com") {
		t.Fatalf("expected Cc header, got=%q", msg)
	}
	if strings.Contains(msg, "\nBcc:") || strings.Contains(msg, "\r\nBcc:") {
		t.Fatalf("expected Bcc header to be omitted, got=%q", msg)
	}
}

func TestBuildRFC822HTMLIncludesAlternativeAndInlineCID(t *testing.T) {
	raw, err := buildRFC822(SendRequest{
		From:     "sender@example.com",
		To:       []string{"to@example.com"},
		Subject:  "HTML",
		BodyHTML: "<p>Hello <strong>world</strong></p>",
		Attachments: []SendAttachment{
			{
				Filename:    "image.png",
				ContentType: "image/png",
				Data:        []byte{1, 2, 3, 4},
				Inline:      true,
				ContentID:   "cid-inline-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRFC822: %v", err)
	}
	msg := string(raw)
	want := []string{
		"multipart/related",
		"multipart/alternative",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Type: text/html; charset=utf-8",
		"Content-Id: <cid-inline-1>",
		"Content-Disposition: inline;",
	}
	for _, token := range want {
		if !strings.Contains(msg, token) {
			t.Fatalf("expected token %q in message: %q", token, msg)
		}
	}
}

func TestBuildRFC822IncludesMessageIDReplyHeadersAndReferences(t *testing.T) {
	raw, err := buildRFC822(SendRequest{
		From:        "sender@example.com",
		To:          []string{"to@example.com"},
		Subject:     "Reply",
		Body:        "Body",
		MessageID:   "new-message@example.com",
		InReplyToID: "orig-message@example.com",
		References:  []string{"older@example.com", "orig-message@example.com"},
	})
	if err != nil {
		t.Fatalf("buildRFC822: %v", err)
	}
	msg := string(raw)
	want := []string{
		"Message-ID: <new-message@example.com>",
		"In-Reply-To: <orig-message@example.com>",
		"References: <older@example.com> <orig-message@example.com>",
	}
	for _, token := range want {
		if !strings.Contains(msg, token) {
			t.Fatalf("expected token %q in message: %q", token, msg)
		}
	}
}

func TestSendWithSenderFallbackUsesToCcBccRecipients(t *testing.T) {
	c := &IMAPSMTPClient{}
	var captured []string
	sendFn := func(ctx context.Context, user, pass, from string, rcpt []string, raw []byte) error {
		captured = append([]string(nil), rcpt...)
		return nil
	}
	req := SendRequest{
		From: "sender@example.com",
		To:   []string{"to@example.com"},
		CC:   []string{"copy@example.com"},
		BCC:  []string{"hidden@example.com"},
	}
	if err := c.sendWithSenderFallback(context.Background(), "sender@example.com", "secret", req, []byte("raw"), sendFn); err != nil {
		t.Fatalf("sendWithSenderFallback: %v", err)
	}
	if len(captured) != 3 {
		t.Fatalf("expected 3 recipients, got %d (%v)", len(captured), captured)
	}
	if captured[0] != "to@example.com" || captured[1] != "copy@example.com" || captured[2] != "hidden@example.com" {
		t.Fatalf("unexpected recipient list: %v", captured)
	}
}
