package mail

import (
	"net/textproto"
	"testing"
)

func TestParsePreferredUnsubscribeActionPrefersHTTPSOneClickPOST(t *testing.T) {
	headers := textproto.MIMEHeader{
		"List-Unsubscribe":      {`<mailto:list@example.com?subject=unsubscribe>, <https://example.test/unsubscribe>`},
		"List-Unsubscribe-Post": {`List-Unsubscribe=One-Click`},
	}

	got := ParsePreferredUnsubscribeAction(headers)
	if got == nil {
		t.Fatal("expected unsubscribe action")
	}
	if got.Method != "POST" {
		t.Fatalf("expected POST method, got %q", got.Method)
	}
	if got.URL != "https://example.test/unsubscribe" {
		t.Fatalf("expected HTTPS unsubscribe URL, got %q", got.URL)
	}
	if !got.OneClick {
		t.Fatal("expected one-click marker")
	}
}

func TestParsePreferredUnsubscribeActionFallsBackToHTTPGet(t *testing.T) {
	headers := textproto.MIMEHeader{
		"List-Unsubscribe": {`<https://example.test/unsubscribe>, <mailto:list@example.com>`},
	}

	got := ParsePreferredUnsubscribeAction(headers)
	if got == nil {
		t.Fatal("expected unsubscribe action")
	}
	if got.Method != "GET" {
		t.Fatalf("expected GET method, got %q", got.Method)
	}
	if got.URL != "https://example.test/unsubscribe" {
		t.Fatalf("expected unsubscribe URL, got %q", got.URL)
	}
}

func TestParsePreferredUnsubscribeActionParsesMailtoFallback(t *testing.T) {
	headers := textproto.MIMEHeader{
		"List-Unsubscribe": {`<mailto:list@example.com?subject=remove&body=please%20unsubscribe>`},
	}

	got := ParsePreferredUnsubscribeAction(headers)
	if got == nil {
		t.Fatal("expected unsubscribe action")
	}
	if got.Method != "MAILTO" {
		t.Fatalf("expected MAILTO method, got %q", got.Method)
	}
	if got.Email != "list@example.com" {
		t.Fatalf("expected unsubscribe email, got %q", got.Email)
	}
	if got.Subject != "remove" {
		t.Fatalf("expected subject, got %q", got.Subject)
	}
	if got.Body != "please unsubscribe" {
		t.Fatalf("expected body, got %q", got.Body)
	}
}
