package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"despatch/internal/mail"
)

type mailHTMLRouterTestClient struct {
	messageByID map[string]mail.Message
}

func (m *mailHTMLRouterTestClient) ListMailboxes(ctx context.Context, user, pass string) ([]mail.Mailbox, error) {
	return []mail.Mailbox{{Name: "INBOX", Unread: 1, Messages: 1}}, nil
}

func (m *mailHTMLRouterTestClient) ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]mail.MessageSummary, error) {
	return []mail.MessageSummary{}, nil
}

func (m *mailHTMLRouterTestClient) GetMessage(ctx context.Context, user, pass, id string) (mail.Message, error) {
	if msg, ok := m.messageByID[id]; ok {
		return msg, nil
	}
	return mail.Message{ID: id}, nil
}

func (m *mailHTMLRouterTestClient) Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]mail.MessageSummary, error) {
	return []mail.MessageSummary{}, nil
}

func (m *mailHTMLRouterTestClient) Send(ctx context.Context, user, pass string, req mail.SendRequest) error {
	return nil
}

func (m *mailHTMLRouterTestClient) SetFlags(ctx context.Context, user, pass, id string, flags []string) error {
	return nil
}

func (m *mailHTMLRouterTestClient) Move(ctx context.Context, user, pass, id, mailbox string) error {
	return nil
}

func (m *mailHTMLRouterTestClient) GetAttachment(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentContent, error) {
	return mail.AttachmentContent{}, nil
}

func (m *mailHTMLRouterTestClient) GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentMeta, io.ReadCloser, error) {
	return mail.AttachmentMeta{}, io.NopCloser(strings.NewReader("")), nil
}

func authedV1GetWithContext(t *testing.T, router http.Handler, ctx context.Context, path string, sessionCookie, csrfCookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = req.WithContext(ctx)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func withRemoteImageClientDialOverride(t *testing.T, upstream *httptest.Server) string {
	t.Helper()
	parsed, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}
	dialAddr := parsed.Host
	prevFactory := remoteImageHTTPClientFactory
	remoteImageHTTPClientFactory = func() *http.Client {
		return &http.Client{
			Timeout: mailRemoteImageFetchTimeout,
			Transport: &http.Transport{
				Proxy: nil,
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, network, dialAddr)
				},
			},
		}
	}
	t.Cleanup(func() {
		remoteImageHTTPClientFactory = prevFactory
	})
	return "http://93.184.216.34"
}

func TestV1GetMessageRewritesCIDAndRemoteImageURLs(t *testing.T) {
	messageID := mail.EncodeMessageID("INBOX", 42)
	router := newSendRouter(t, &mailHTMLRouterTestClient{
		messageByID: map[string]mail.Message{
			messageID: {
				ID:       messageID,
				Body:     "fallback",
				BodyHTML: `<html><body><img src="cid:Logo.CID"><img src="https://cdn.example.com/banner.png"></body></html>`,
				Attachments: []mail.AttachmentMeta{
					{ID: "att-inline", Inline: true, ContentID: "logo.cid", ContentType: "image/png"},
				},
			},
		},
	}, "")

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v1/messages/"+url.PathEscape(messageID), sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload mail.Message
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v body=%s", err, rec.Body.String())
	}
	if !strings.Contains(payload.BodyHTML, "/api/v1/attachments/att-inline") {
		t.Fatalf("expected cid image to be rewritten to attachment endpoint, got %q", payload.BodyHTML)
	}
	if strings.Contains(payload.BodyHTML, "https://cdn.example.com/banner.png") {
		t.Fatalf("expected remote image URL to be rewritten through proxy, got %q", payload.BodyHTML)
	}
	if !strings.Contains(payload.BodyHTML, "/api/v1/messages/") || !strings.Contains(payload.BodyHTML, "/remote-image?url=") {
		t.Fatalf("expected remote image proxy URL in html, got %q", payload.BodyHTML)
	}
}

func TestV1RemoteImageProxyAcceptsImage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{1, 2, 3, 4})
	}))
	defer upstream.Close()
	targetBase := withRemoteImageClientDialOverride(t, upstream)

	router := newSendRouter(t, &sendTestDespatch{}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)
	messageID := mail.EncodeMessageID("INBOX", 7)
	path := fmt.Sprintf("/api/v1/messages/%s/remote-image?url=%s", url.PathEscape(messageID), url.QueryEscape(targetBase+"/pixel.png"))
	rec := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := strings.ToLower(rec.Header().Get("Content-Type")); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("expected image/png content-type, got %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), []byte{1, 2, 3, 4}) {
		t.Fatalf("unexpected proxied body: %v", rec.Body.Bytes())
	}
	if cache := rec.Header().Get("Cache-Control"); !strings.Contains(cache, "private") {
		t.Fatalf("expected private cache control, got %q", cache)
	}
}

func TestV1RemoteImageProxyRejectsPrivateTargets(t *testing.T) {
	router := newSendRouter(t, &sendTestDespatch{}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)
	messageID := mail.EncodeMessageID("INBOX", 8)
	path := fmt.Sprintf("/api/v1/messages/%s/remote-image?url=%s", url.PathEscape(messageID), url.QueryEscape("http://127.0.0.1/private.png"))
	rec := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestV1RemoteImageProxyRejectsNonImageContentType(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>nope</html>"))
	}))
	defer upstream.Close()
	targetBase := withRemoteImageClientDialOverride(t, upstream)

	router := newSendRouter(t, &sendTestDespatch{}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)
	messageID := mail.EncodeMessageID("INBOX", 9)
	path := fmt.Sprintf("/api/v1/messages/%s/remote-image?url=%s", url.PathEscape(messageID), url.QueryEscape(targetBase+"/not-image"))
	rec := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestV1RemoteImageProxyEnforcesMaxSize(t *testing.T) {
	large := bytes.Repeat([]byte{0xaa}, int(mailRemoteImageMaxBytes+1))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(large)))
		_, _ = w.Write(large)
	}))
	defer upstream.Close()
	targetBase := withRemoteImageClientDialOverride(t, upstream)

	router := newSendRouter(t, &sendTestDespatch{}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)
	messageID := mail.EncodeMessageID("INBOX", 10)
	path := fmt.Sprintf("/api/v1/messages/%s/remote-image?url=%s", url.PathEscape(messageID), url.QueryEscape(targetBase+"/big.png"))
	rec := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestV1RemoteImageProxyHonorsContextTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{1, 2, 3})
	}))
	defer upstream.Close()
	targetBase := withRemoteImageClientDialOverride(t, upstream)

	router := newSendRouter(t, &sendTestDespatch{}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)
	messageID := mail.EncodeMessageID("INBOX", 11)
	path := fmt.Sprintf("/api/v1/messages/%s/remote-image?url=%s", url.PathEscape(messageID), url.QueryEscape(targetBase+"/slow.png"))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	rec := authedV1GetWithContext(t, router, ctx, path, sessionCookie, csrfCookie)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for timed-out upstream fetch, got %d body=%s", rec.Code, rec.Body.String())
	}
}
