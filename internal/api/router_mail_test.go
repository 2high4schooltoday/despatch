package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"despatch/internal/mail"
)

type mailRouterTestClient struct {
	listByPage        map[int][]mail.MessageSummary
	listByMailboxPage map[string]map[int][]mail.MessageSummary
	search            []mail.MessageSummary
	mailboxes         []mail.Mailbox
}

func (m *mailRouterTestClient) ListMailboxes(ctx context.Context, user, pass string) ([]mail.Mailbox, error) {
	if len(m.mailboxes) > 0 {
		return m.mailboxes, nil
	}
	return []mail.Mailbox{{Name: "INBOX", Unread: 1, Messages: 3}}, nil
}

func (m *mailRouterTestClient) ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]mail.MessageSummary, error) {
	if m.listByMailboxPage != nil {
		if byPage, ok := m.listByMailboxPage[mailbox]; ok {
			if items, ok := byPage[page]; ok {
				return items, nil
			}
			return []mail.MessageSummary{}, nil
		}
	}
	if m.listByPage == nil {
		return []mail.MessageSummary{}, nil
	}
	if items, ok := m.listByPage[page]; ok {
		return items, nil
	}
	return []mail.MessageSummary{}, nil
}

func (m *mailRouterTestClient) GetMessage(ctx context.Context, user, pass, id string) (mail.Message, error) {
	return mail.Message{ID: id}, nil
}

func (m *mailRouterTestClient) Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]mail.MessageSummary, error) {
	return m.search, nil
}

func (m *mailRouterTestClient) Send(ctx context.Context, user, pass string, req mail.SendRequest) (mail.SendResult, error) {
	return mail.SendResult{SavedCopy: true, SavedCopyMailbox: "Sent"}, nil
}

func (m *mailRouterTestClient) SetFlags(ctx context.Context, user, pass, id string, flags []string) error {
	return nil
}

func (m *mailRouterTestClient) UpdateFlags(ctx context.Context, user, pass, id string, patch mail.FlagPatch) error {
	return nil
}

func (m *mailRouterTestClient) Move(ctx context.Context, user, pass, id, mailbox string) error {
	return nil
}

func (m *mailRouterTestClient) GetAttachment(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentContent, error) {
	return mail.AttachmentContent{}, nil
}

func (m *mailRouterTestClient) GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (mail.AttachmentMeta, io.ReadCloser, error) {
	return mail.AttachmentMeta{}, io.NopCloser(strings.NewReader("")), nil
}

func authedV1Get(t *testing.T, router http.Handler, path string, sessionCookie, csrfCookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(sessionCookie)
	req.AddCookie(csrfCookie)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestV1ListMessagesIncludesPreviewAndThreadID(t *testing.T) {
	threadID := mail.DeriveThreadID("INBOX", "Status Update", "alice@example.com")
	router := newSendRouter(t, &mailRouterTestClient{
		listByPage: map[int][]mail.MessageSummary{
			1: {{
				ID:       "msg-1",
				From:     "alice@example.com",
				Subject:  "Status Update",
				Preview:  "Deployment completed successfully.",
				ThreadID: threadID,
			}},
		},
	}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v1/messages?mailbox=INBOX&page=1&page_size=25", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []mail.MessageSummary `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list messages: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(payload.Items))
	}
	if payload.Items[0].Preview == "" || payload.Items[0].ThreadID == "" {
		t.Fatalf("expected preview and thread_id in payload item: %+v", payload.Items[0])
	}
}

func TestV1SearchIncludesPreviewAndThreadID(t *testing.T) {
	threadID := mail.DeriveThreadID("INBOX", "Roadmap", "bob@example.com")
	router := newSendRouter(t, &mailRouterTestClient{
		search: []mail.MessageSummary{{
			ID:       "search-1",
			From:     "bob@example.com",
			Subject:  "Roadmap",
			Preview:  "Q2 priorities draft attached.",
			ThreadID: threadID,
		}},
	}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v1/search?mailbox=INBOX&q=roadmap&page=1&page_size=25", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []mail.MessageSummary `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode search payload: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Items) != 1 {
		t.Fatalf("expected 1 search item, got %d", len(payload.Items))
	}
	if payload.Items[0].Preview == "" || payload.Items[0].ThreadID == "" {
		t.Fatalf("expected preview and thread_id in search item: %+v", payload.Items[0])
	}
}

func TestV1ListMailboxesIncludesRole(t *testing.T) {
	router := newSendRouter(t, &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 2, Messages: 7},
			{Name: "Sent Messages", Role: "sent", Unread: 0, Messages: 4},
			{Name: "Deleted Messages", Role: "trash", Unread: 0, Messages: 1},
		},
	}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	rec := authedV1Get(t, router, "/api/v1/mailboxes", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload []mail.Mailbox
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode mailboxes payload: %v body=%s", err, rec.Body.String())
	}
	if len(payload) != 3 {
		t.Fatalf("expected 3 mailboxes, got %+v", payload)
	}
	if payload[1].Role != "sent" || payload[2].Role != "trash" {
		t.Fatalf("expected roles in payload, got %+v", payload)
	}
}

func TestV1ThreadMessagesFiltersAndPaginates(t *testing.T) {
	threadID := mail.DeriveThreadID("INBOX", "Release Plan", "alice@example.com")
	otherThreadID := mail.DeriveThreadID("INBOX", "Unrelated", "charlie@example.com")
	router := newSendRouter(t, &mailRouterTestClient{
		listByPage: map[int][]mail.MessageSummary{
			1: {
				{ID: "m1", Subject: "Re: Release Plan", From: "alice@example.com", ThreadID: threadID},
				{ID: "m2", Subject: "Unrelated", From: "charlie@example.com", ThreadID: otherThreadID},
			},
			2: {
				{ID: "m3", Subject: "Fwd: Release Plan", From: "alice@example.com", ThreadID: threadID},
			},
			3: {},
		},
	}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	path := "/api/v1/threads/" + url.PathEscape(threadID) + "/messages?mailbox=INBOX&page=1&page_size=2"
	rec := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		ThreadID         string                `json:"thread_id"`
		Scope            string                `json:"scope"`
		MailboxesScanned []string              `json:"mailboxes_scanned"`
		Truncated        bool                  `json:"truncated"`
		Items            []mail.MessageSummary `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode thread payload: %v body=%s", err, rec.Body.String())
	}
	if payload.ThreadID != threadID {
		t.Fatalf("expected thread_id %q, got %q", threadID, payload.ThreadID)
	}
	if payload.Scope != "mailbox" {
		t.Fatalf("expected mailbox scope, got %q", payload.Scope)
	}
	if len(payload.MailboxesScanned) != 1 || payload.MailboxesScanned[0] != "INBOX" {
		t.Fatalf("unexpected mailboxes_scanned: %+v", payload.MailboxesScanned)
	}
	if payload.Truncated {
		t.Fatalf("expected truncated=false")
	}
	if len(payload.Items) != 2 || payload.Items[0].ID != "m1" || payload.Items[1].ID != "m3" {
		t.Fatalf("unexpected filtered items: %+v", payload.Items)
	}

	path = "/api/v1/threads/" + url.PathEscape(threadID) + "/messages?mailbox=INBOX&page=2&page_size=1"
	rec = authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	payload = struct {
		ThreadID         string                `json:"thread_id"`
		Scope            string                `json:"scope"`
		MailboxesScanned []string              `json:"mailboxes_scanned"`
		Truncated        bool                  `json:"truncated"`
		Items            []mail.MessageSummary `json:"items"`
	}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode paginated thread payload: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != "m3" {
		t.Fatalf("expected second thread item m3, got %+v", payload.Items)
	}
}

func TestV1ThreadMessagesConversationScopeScansDefaultFolders(t *testing.T) {
	threadID := mail.DeriveThreadID("INBOX", "Release Plan", "alice@example.com")
	router := newSendRouter(t, &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 1, Messages: 10},
			{Name: "Sent Messages", Role: "sent", Unread: 0, Messages: 5},
			{Name: "All Mail", Role: "archive", Unread: 0, Messages: 6},
			{Name: "Deleted Messages", Role: "trash", Unread: 0, Messages: 2},
		},
		listByMailboxPage: map[string]map[int][]mail.MessageSummary{
			"INBOX": {
				1: {
					{ID: "i1", Mailbox: "INBOX", Subject: "Re: Release Plan", From: "alice@example.com", ThreadID: threadID},
				},
				2: {},
			},
			"Sent Messages": {
				1: {
					{ID: "s1", Mailbox: "Sent Messages", Subject: "Release Plan", From: "alice@example.com", ThreadID: threadID},
				},
				2: {},
			},
			"All Mail": {
				1: {
					{ID: "a1", Mailbox: "All Mail", Subject: "Fwd: Release Plan", From: "alice@example.com", ThreadID: threadID},
				},
				2: {},
			},
			"Deleted Messages": {
				1: {
					{ID: "t1", Mailbox: "Deleted Messages", Subject: "Release Plan", From: "alice@example.com", ThreadID: threadID},
				},
				2: {},
			},
		},
	}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	path := "/api/v1/threads/" + url.PathEscape(threadID) + "/messages?mailbox=INBOX&scope=conversation&page=1&page_size=10"
	rec := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Scope            string                `json:"scope"`
		MailboxesScanned []string              `json:"mailboxes_scanned"`
		Items            []mail.MessageSummary `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode conversation payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Scope != "conversation" {
		t.Fatalf("expected conversation scope, got %q", payload.Scope)
	}
	if len(payload.MailboxesScanned) < 3 {
		t.Fatalf("expected Inbox/Sent/Archive scan set, got %+v", payload.MailboxesScanned)
	}
	if len(payload.Items) != 3 {
		t.Fatalf("expected 3 cross-mailbox thread items, got %+v", payload.Items)
	}
	for _, item := range payload.Items {
		if item.Mailbox == "Deleted Messages" {
			t.Fatalf("did not expect trash mailbox in default conversation scan: %+v", payload.Items)
		}
	}
}

func TestV1ThreadMessagesSetsTruncatedWhenScanCapReached(t *testing.T) {
	listByPage := make(map[int][]mail.MessageSummary, threadMessagesMaxScanPages)
	otherThreadID := mail.DeriveThreadID("INBOX", "Other", "x@example.com")
	for i := 1; i <= threadMessagesMaxScanPages; i++ {
		listByPage[i] = []mail.MessageSummary{{
			ID:       "m",
			From:     "x@example.com",
			Subject:  "Other",
			ThreadID: otherThreadID,
		}}
	}
	router := newSendRouter(t, &mailRouterTestClient{listByPage: listByPage}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	targetThread := mail.DeriveThreadID("INBOX", "Missing", "nobody@example.com")
	path := "/api/v1/threads/" + url.PathEscape(targetThread) + "/messages?mailbox=INBOX&page=1&page_size=25"
	rec := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Truncated bool                  `json:"truncated"`
		Items     []mail.MessageSummary `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode truncated payload: %v body=%s", err, rec.Body.String())
	}
	if !payload.Truncated {
		t.Fatalf("expected truncated=true when scan cap reached")
	}
	if len(payload.Items) != 0 {
		t.Fatalf("expected no items, got %+v", payload.Items)
	}
}
