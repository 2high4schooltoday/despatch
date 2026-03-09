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
	createdMailboxes  []string
	listMailboxCalls  int
	listMessageCalls  int
}

func (m *mailRouterTestClient) ListMailboxes(ctx context.Context, user, pass string) ([]mail.Mailbox, error) {
	m.listMailboxCalls++
	if len(m.mailboxes) > 0 {
		return m.mailboxes, nil
	}
	return []mail.Mailbox{{Name: "INBOX", Unread: 1, Messages: 3}}, nil
}

func (m *mailRouterTestClient) CreateMailbox(ctx context.Context, user, pass, mailbox string) error {
	m.createdMailboxes = append(m.createdMailboxes, mailbox)
	m.mailboxes = append(m.mailboxes, mail.Mailbox{Name: mailbox})
	return nil
}

func (m *mailRouterTestClient) ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]mail.MessageSummary, error) {
	m.listMessageCalls++
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

func TestV1ListMailboxesUsesCacheUntilInvalidated(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 2, Messages: 7},
			{Name: "Team Sent", Unread: 0, Messages: 0},
		},
	}
	router, _, _ := newSendRouterWithStore(t, client, "account@example.com")
	sessionCookie, csrfCookie := loginForSend(t, router)

	first := authedV1Get(t, router, "/api/v1/mailboxes", sessionCookie, csrfCookie)
	if first.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", first.Code, first.Body.String())
	}
	second := authedV1Get(t, router, "/api/v1/mailboxes", sessionCookie, csrfCookie)
	if second.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", second.Code, second.Body.String())
	}
	if client.listMailboxCalls != 1 {
		t.Fatalf("expected mailbox list to be cached, got %d underlying calls", client.listMailboxCalls)
	}

	rec := postMailJSON(t, router, "/api/v1/mailboxes/special/sent", sessionCookie, csrfCookie, []byte(`{"mailbox_name":"Team Sent","create_if_missing":false}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if client.listMailboxCalls != 1 {
		t.Fatalf("expected special mailbox update to reuse cached raw mailboxes, got %d calls", client.listMailboxCalls)
	}

	afterInvalidate := authedV1Get(t, router, "/api/v1/mailboxes", sessionCookie, csrfCookie)
	if afterInvalidate.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", afterInvalidate.Code, afterInvalidate.Body.String())
	}
	if client.listMailboxCalls != 2 {
		t.Fatalf("expected mailbox cache invalidation after special mailbox update, got %d calls", client.listMailboxCalls)
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

func TestV1ThreadMessagesUsesCacheAndInvalidatesOnFlagChange(t *testing.T) {
	threadID := mail.DeriveThreadID("INBOX", "Release Plan", "alice@example.com")
	client := &mailRouterTestClient{
		listByPage: map[int][]mail.MessageSummary{
			1: {
				{ID: "m1", Subject: "Release Plan", From: "alice@example.com", ThreadID: threadID},
			},
			2: {},
		},
	}
	router := newSendRouter(t, client, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	path := "/api/v1/threads/" + url.PathEscape(threadID) + "/messages?mailbox=INBOX&page=1&page_size=10"
	first := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if first.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", first.Code, first.Body.String())
	}
	if client.listMessageCalls != 2 {
		t.Fatalf("expected initial thread lookup to scan two pages, got %d calls", client.listMessageCalls)
	}

	second := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if second.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", second.Code, second.Body.String())
	}
	if client.listMessageCalls != 2 {
		t.Fatalf("expected cached thread lookup to avoid extra IMAP calls, got %d calls", client.listMessageCalls)
	}

	flagsRec := postMailJSON(t, router, "/api/v1/messages/m1/flags", sessionCookie, csrfCookie, []byte(`{"add":["\\Seen"]}`))
	if flagsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", flagsRec.Code, flagsRec.Body.String())
	}

	afterInvalidate := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if afterInvalidate.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", afterInvalidate.Code, afterInvalidate.Body.String())
	}
	if client.listMessageCalls != 4 {
		t.Fatalf("expected thread cache invalidation after flag update, got %d calls", client.listMessageCalls)
	}
}

func TestV1ThreadMessagesConversationScopeScansAllNonDraftFolders(t *testing.T) {
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
		t.Fatalf("expected multiple mailboxes scanned, got %+v", payload.MailboxesScanned)
	}
	if len(payload.Items) != 4 {
		t.Fatalf("expected 4 cross-mailbox thread items, got %+v", payload.Items)
	}
	if payload.Items[len(payload.Items)-1].Mailbox != "Deleted Messages" {
		t.Fatalf("expected trash mailbox to be scanned after primary folders, got %+v", payload.Items)
	}
}

func TestV1ThreadMessagesConversationScopeIncludesCustomFolders(t *testing.T) {
	threadID := mail.DeriveThreadID("INBOX", "Project Check-In", "alice@example.com")
	router := newSendRouter(t, &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox"},
			{Name: "Sent Items", Role: "sent"},
			{Name: "Projects/Alpha"},
			{Name: "Drafts", Role: "drafts"},
			{Name: "Spam", Role: "junk"},
		},
		listByMailboxPage: map[string]map[int][]mail.MessageSummary{
			"INBOX":          {1: {}, 2: {}},
			"Sent Items":     {1: {}, 2: {}},
			"Projects/Alpha": {1: {{ID: "p1", Mailbox: "Projects/Alpha", Subject: "Re: Project Check-In", From: "alice@example.com", ThreadID: threadID}}, 2: {}},
			"Spam":           {1: {}, 2: {}},
		},
	}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	path := "/api/v1/threads/" + url.PathEscape(threadID) + "/messages?mailbox=INBOX&scope=conversation&page=1&page_size=10"
	rec := authedV1Get(t, router, path, sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		MailboxesScanned []string              `json:"mailboxes_scanned"`
		Items            []mail.MessageSummary `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode conversation payload: %v body=%s", err, rec.Body.String())
	}
	if !containsString(payload.MailboxesScanned, "Projects/Alpha") {
		t.Fatalf("expected custom folder in scan list, got %+v", payload.MailboxesScanned)
	}
	if containsString(payload.MailboxesScanned, "Drafts") {
		t.Fatalf("did not expect drafts mailbox in scan list, got %+v", payload.MailboxesScanned)
	}
	if len(payload.Items) != 1 || payload.Items[0].Mailbox != "Projects/Alpha" {
		t.Fatalf("expected custom-folder thread result, got %+v", payload.Items)
	}
}

func TestV1SpecialMailboxMappingCreatesAndPersistsFolder(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 2, Messages: 7},
		},
	}
	router, st, _ := newSendRouterWithStore(t, client, "account@example.com")
	sessionCookie, csrfCookie := loginForSend(t, router)

	rec := postMailJSON(t, router, "/api/v1/mailboxes/special/archive", sessionCookie, csrfCookie, []byte(`{"mailbox_name":"Archive","create_if_missing":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(client.createdMailboxes) != 1 || client.createdMailboxes[0] != "Archive" {
		t.Fatalf("expected mailbox creation to run once, got %+v", client.createdMailboxes)
	}

	admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	mappings, err := st.ListSpecialMailboxMappings(context.Background(), admin.ID, "account@example.com")
	if err != nil {
		t.Fatalf("load special mappings: %v", err)
	}
	if mappings["archive"] != "Archive" {
		t.Fatalf("expected persisted archive mapping, got %+v", mappings)
	}

	mailboxesRec := authedV1Get(t, router, "/api/v1/mailboxes", sessionCookie, csrfCookie)
	if mailboxesRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", mailboxesRec.Code, mailboxesRec.Body.String())
	}
	var mailboxes []mail.Mailbox
	if err := json.Unmarshal(mailboxesRec.Body.Bytes(), &mailboxes); err != nil {
		t.Fatalf("decode mailboxes: %v", err)
	}
	if resolved := mail.ResolveMailboxByRole(mailboxes, "archive"); resolved != "Archive" {
		t.Fatalf("expected archive role overlay, got %+v", mailboxes)
	}

	specialRec := authedV1Get(t, router, "/api/v1/mailboxes/special", sessionCookie, csrfCookie)
	if specialRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", specialRec.Code, specialRec.Body.String())
	}
	var payload struct {
		Items []specialMailboxMappingDTO `json:"items"`
	}
	if err := json.Unmarshal(specialRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode special mailboxes payload: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].Role != "archive" || payload.Items[0].MailboxName != "Archive" {
		t.Fatalf("unexpected special mailbox payload: %+v", payload.Items)
	}
}

func TestV1SpecialMailboxMappingSupportsSentRole(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 2, Messages: 7},
			{Name: "Sent Items", Role: "sent", Unread: 0, Messages: 3},
			{Name: "Team Sent", Unread: 0, Messages: 0},
		},
	}
	router, st, _ := newSendRouterWithStore(t, client, "account@example.com")
	sessionCookie, csrfCookie := loginForSend(t, router)

	rec := postMailJSON(t, router, "/api/v1/mailboxes/special/sent", sessionCookie, csrfCookie, []byte(`{"mailbox_name":"Team Sent","create_if_missing":false}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	admin, err := st.GetUserByEmail(context.Background(), "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	mappings, err := st.ListSpecialMailboxMappings(context.Background(), admin.ID, "account@example.com")
	if err != nil {
		t.Fatalf("load special mappings: %v", err)
	}
	if mappings["sent"] != "Team Sent" {
		t.Fatalf("expected persisted sent mapping, got %+v", mappings)
	}

	mailboxesRec := authedV1Get(t, router, "/api/v1/mailboxes", sessionCookie, csrfCookie)
	if mailboxesRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", mailboxesRec.Code, mailboxesRec.Body.String())
	}
	var mailboxes []mail.Mailbox
	if err := json.Unmarshal(mailboxesRec.Body.Bytes(), &mailboxes); err != nil {
		t.Fatalf("decode mailboxes: %v", err)
	}
	if resolved := mail.ResolveMailboxByRole(mailboxes, "sent"); resolved != "Team Sent" {
		t.Fatalf("expected sent role overlay, got %+v", mailboxes)
	}

	specialRec := authedV1Get(t, router, "/api/v1/mailboxes/special", sessionCookie, csrfCookie)
	if specialRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", specialRec.Code, specialRec.Body.String())
	}
	var payload struct {
		Items []specialMailboxMappingDTO `json:"items"`
	}
	if err := json.Unmarshal(specialRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode special mailboxes payload: %v", err)
	}
	if !containsSpecialMailbox(payload.Items, "sent", "Team Sent") {
		t.Fatalf("expected sent mapping in payload, got %+v", payload.Items)
	}
}

func containsSpecialMailbox(items []specialMailboxMappingDTO, role, mailboxName string) bool {
	for _, item := range items {
		if item.Role == role && item.MailboxName == mailboxName {
			return true
		}
	}
	return false
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
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
