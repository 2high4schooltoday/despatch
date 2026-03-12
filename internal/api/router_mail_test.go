package api

import (
	"bytes"
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
	messageByID       map[string]mail.Message
	rawByID           map[string][]byte
	createdMailboxes  []string
	renamedMailboxes  [][2]string
	deletedMailboxes  []string
	movedMessages     map[string]string
	createMailboxErr  error
	renameMailboxErr  error
	deleteMailboxErr  error
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
	if m.createMailboxErr != nil {
		return m.createMailboxErr
	}
	m.createdMailboxes = append(m.createdMailboxes, mailbox)
	m.mailboxes = append(m.mailboxes, mail.Mailbox{Name: mailbox})
	return nil
}

func (m *mailRouterTestClient) RenameMailbox(ctx context.Context, user, pass, mailbox, newMailbox string) error {
	if m.renameMailboxErr != nil {
		return m.renameMailboxErr
	}
	m.renamedMailboxes = append(m.renamedMailboxes, [2]string{mailbox, newMailbox})
	for i := range m.mailboxes {
		if strings.EqualFold(strings.TrimSpace(m.mailboxes[i].Name), strings.TrimSpace(mailbox)) {
			m.mailboxes[i].Name = newMailbox
			break
		}
	}
	return nil
}

func (m *mailRouterTestClient) DeleteMailbox(ctx context.Context, user, pass, mailbox string) error {
	if m.deleteMailboxErr != nil {
		return m.deleteMailboxErr
	}
	m.deletedMailboxes = append(m.deletedMailboxes, mailbox)
	filtered := m.mailboxes[:0]
	for _, item := range m.mailboxes {
		if strings.EqualFold(strings.TrimSpace(item.Name), strings.TrimSpace(mailbox)) {
			continue
		}
		filtered = append(filtered, item)
	}
	m.mailboxes = filtered
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
	if msg, ok := m.messageByID[id]; ok {
		return msg, nil
	}
	return mail.Message{ID: id}, nil
}

func (m *mailRouterTestClient) GetRawMessage(ctx context.Context, user, pass, id string) ([]byte, error) {
	if raw, ok := m.rawByID[id]; ok {
		return append([]byte(nil), raw...), nil
	}
	return []byte("From: test@example.com\r\n\r\nbody"), nil
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
	if m.movedMessages == nil {
		m.movedMessages = map[string]string{}
	}
	m.movedMessages[id] = mailbox
	removeSummary := func(items []mail.MessageSummary) []mail.MessageSummary {
		filtered := items[:0]
		for _, item := range items {
			if strings.TrimSpace(item.ID) == strings.TrimSpace(id) {
				continue
			}
			filtered = append(filtered, item)
		}
		return filtered
	}
	if sourceMailbox, _, err := mail.DecodeMessageID(id); err == nil {
		for i := range m.mailboxes {
			switch {
			case strings.EqualFold(strings.TrimSpace(m.mailboxes[i].Name), strings.TrimSpace(sourceMailbox)) && m.mailboxes[i].Messages > 0:
				m.mailboxes[i].Messages--
			case strings.EqualFold(strings.TrimSpace(m.mailboxes[i].Name), strings.TrimSpace(mailbox)):
				m.mailboxes[i].Messages++
			}
		}
	}
	if m.listByMailboxPage != nil {
		for _, byPage := range m.listByMailboxPage {
			for page, items := range byPage {
				byPage[page] = removeSummary(items)
			}
		}
	}
	if m.listByPage != nil {
		for page, items := range m.listByPage {
			m.listByPage[page] = removeSummary(items)
		}
	}
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

func authedV1JSON(t *testing.T, router http.Handler, method, path string, sessionCookie, csrfCookie *http.Cookie, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
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
	if payload.Items[0].Preview == "" || payload.Items[0].ThreadID == "" || payload.Items[0].Source != "live" {
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
	if payload.Items[0].Preview == "" || payload.Items[0].ThreadID == "" || payload.Items[0].Source != "live" {
		t.Fatalf("expected preview and thread_id in search item: %+v", payload.Items[0])
	}
}

func TestV1GetMessageIncludesLiveSource(t *testing.T) {
	messageID := mail.EncodeMessageID("INBOX", 42)
	router := newSendRouter(t, &mailRouterTestClient{
		messageByID: map[string]mail.Message{
			messageID: {
				ID:      messageID,
				Mailbox: "INBOX",
				From:    "alice@example.com",
				To:      []string{"bob@example.com"},
				Subject: "Detail",
				Body:    "hello",
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
		t.Fatalf("decode message payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Source != "live" {
		t.Fatalf("expected source=live, got %+v", payload)
	}
}

func TestV1ListMailboxesIncludesRole(t *testing.T) {
	router := newSendRouter(t, &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 2, Messages: 7},
			{Name: "Sent Messages", Role: "sent", Unread: 0, Messages: 4},
			{Name: "Deleted Messages", Role: "trash", Unread: 0, Messages: 1},
			{Name: "Projects", Unread: 0, Messages: 0},
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
	if len(payload) != 4 {
		t.Fatalf("expected 4 mailboxes, got %+v", payload)
	}
	if payload[1].Role != "sent" || payload[2].Role != "trash" {
		t.Fatalf("expected roles in payload, got %+v", payload)
	}
	if payload[0].CanRename || payload[0].CanDelete {
		t.Fatalf("expected inbox capabilities disabled, got %+v", payload[0])
	}
	if !payload[3].CanRename || !payload[3].CanDelete {
		t.Fatalf("expected custom folder capabilities enabled, got %+v", payload[3])
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

func TestV1CreateMailboxReturns201AndInvalidatesCache(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 2, Messages: 7},
		},
	}
	router, _, _ := newSendRouterWithStore(t, client, "account@example.com")
	sessionCookie, csrfCookie := loginForSend(t, router)

	first := authedV1Get(t, router, "/api/v1/mailboxes", sessionCookie, csrfCookie)
	if first.Code != http.StatusOK {
		t.Fatalf("expected initial 200, got %d body=%s", first.Code, first.Body.String())
	}

	create := authedV1JSON(t, router, http.MethodPost, "/api/v1/mailboxes", sessionCookie, csrfCookie, []byte(`{"mailbox_name":"Projects"}`))
	if create.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", create.Code, create.Body.String())
	}
	if len(client.createdMailboxes) != 1 || client.createdMailboxes[0] != "Projects" {
		t.Fatalf("expected Projects mailbox creation, got %+v", client.createdMailboxes)
	}
	if client.listMailboxCalls != 2 {
		t.Fatalf("expected create to invalidate and reload cached mailboxes, got %d calls", client.listMailboxCalls)
	}

	after := authedV1Get(t, router, "/api/v1/mailboxes", sessionCookie, csrfCookie)
	if after.Code != http.StatusOK {
		t.Fatalf("expected follow-up 200, got %d body=%s", after.Code, after.Body.String())
	}
	if client.listMailboxCalls != 2 {
		t.Fatalf("expected follow-up list to use refreshed cache, got %d calls", client.listMailboxCalls)
	}
}

func TestV1RenameMailboxSuccess(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 1, Messages: 5},
			{Name: "Projects", Unread: 0, Messages: 0},
		},
	}
	router := newSendRouter(t, client, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	rename := authedV1JSON(t, router, http.MethodPatch, "/api/v1/mailboxes", sessionCookie, csrfCookie, []byte(`{"mailbox_name":"Projects","new_mailbox_name":"Projects/2026"}`))
	if rename.Code != http.StatusOK {
		t.Fatalf("expected rename 200, got %d body=%s", rename.Code, rename.Body.String())
	}
	if len(client.renamedMailboxes) != 1 || client.renamedMailboxes[0][0] != "Projects" || client.renamedMailboxes[0][1] != "Projects/2026" {
		t.Fatalf("expected rename call, got %+v", client.renamedMailboxes)
	}
}

func TestV1RenameMailboxRejectsProtected(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 1, Messages: 5},
			{Name: "Sent", Role: "sent", Unread: 0, Messages: 0},
		},
	}
	router := newSendRouter(t, client, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	rename := authedV1JSON(t, router, http.MethodPatch, "/api/v1/mailboxes", sessionCookie, csrfCookie, []byte(`{"mailbox_name":"Sent","new_mailbox_name":"Sent Archive"}`))
	if rename.Code != http.StatusBadRequest {
		t.Fatalf("expected rename 400, got %d body=%s", rename.Code, rename.Body.String())
	}
	if !strings.Contains(rename.Body.String(), "mailbox_protected") {
		t.Fatalf("expected mailbox_protected error, got body=%s", rename.Body.String())
	}
}

func TestV1DeleteMailboxMovesMessagesToTrashThenDeletes(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 1, Messages: 5},
			{Name: "Trash", Role: "trash", Unread: 0, Messages: 0},
			{Name: "Projects", Unread: 0, Messages: 2},
		},
		listByMailboxPage: map[string]map[int][]mail.MessageSummary{
			"Projects": {
				1: {
					{ID: mail.EncodeMessageID("Projects", 1), Subject: "One"},
					{ID: mail.EncodeMessageID("Projects", 2), Subject: "Two"},
				},
			},
		},
	}
	router := newSendRouter(t, client, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	del := authedV1JSON(t, router, http.MethodDelete, "/api/v1/mailboxes", sessionCookie, csrfCookie, []byte(`{"mailbox_name":"Projects"}`))
	if del.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d body=%s", del.Code, del.Body.String())
	}
	if len(client.deletedMailboxes) != 1 || client.deletedMailboxes[0] != "Projects" {
		t.Fatalf("expected Projects mailbox to be deleted, got %+v", client.deletedMailboxes)
	}
	if len(client.movedMessages) != 2 {
		t.Fatalf("expected messages moved to trash before delete, got %+v", client.movedMessages)
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
	if payload.Items[0].Source != "live" || payload.Items[1].Source != "live" {
		t.Fatalf("expected live source in thread items, got %+v", payload.Items)
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

func TestV1GetMessageRawReturnsRFC822(t *testing.T) {
	messageID := mail.EncodeMessageID("INBOX", 42)
	router := newSendRouter(t, &mailRouterTestClient{
		rawByID: map[string][]byte{
			messageID: []byte("From: sender@example.com\r\nSubject: Raw\r\n\r\nbody"),
		},
	}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	rec := authedV1Get(t, router, "/api/v1/messages/"+url.PathEscape(messageID)+"/raw", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "message/rfc822" {
		t.Fatalf("expected message/rfc822 content-type, got %q", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Subject: Raw") {
		t.Fatalf("expected raw RFC822 body, got %q", body)
	}
}

func TestV1GetMessageRawDownloadSetsAttachmentFilename(t *testing.T) {
	messageID := mail.EncodeMessageID("INBOX", 7)
	router := newSendRouter(t, &mailRouterTestClient{
		rawByID: map[string][]byte{
			messageID: []byte("From: sender@example.com\r\n\r\nbody"),
		},
	}, "")
	sessionCookie, csrfCookie := loginForSend(t, router)

	rec := authedV1Get(t, router, "/api/v1/messages/"+url.PathEscape(messageID)+"/raw?download=1", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.Contains(strings.ToLower(got), "attachment") || !strings.Contains(strings.ToLower(got), ".eml") {
		t.Fatalf("expected attachment .eml content-disposition, got %q", got)
	}
}

func TestV1GetMessageRawRequiresAuth(t *testing.T) {
	messageID := mail.EncodeMessageID("INBOX", 9)
	router := newSendRouter(t, &mailRouterTestClient{}, "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/messages/"+url.PathEscape(messageID)+"/raw", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}
