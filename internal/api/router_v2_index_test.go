package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"despatch/internal/auth"
	"despatch/internal/config"
	"despatch/internal/db"
	"despatch/internal/mail"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/util"
)

type indexedBulkTestMailClient struct {
	mail.NoopClient
	mu      sync.Mutex
	patches map[string]mail.FlagPatch
	moves   map[string]string
}

func (m *indexedBulkTestMailClient) UpdateFlags(ctx context.Context, user, pass, id string, patch mail.FlagPatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.patches == nil {
		m.patches = map[string]mail.FlagPatch{}
	}
	m.patches[id] = patch
	return nil
}

func (m *indexedBulkTestMailClient) Move(ctx context.Context, user, pass, id, mailbox string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.moves == nil {
		m.moves = map[string]string{}
	}
	m.moves[id] = mailbox
	return nil
}

func (m *indexedBulkTestMailClient) patchFor(id string) (mail.FlagPatch, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	patch, ok := m.patches[id]
	return patch, ok
}

func (m *indexedBulkTestMailClient) moveFor(id string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.moves[id]
	return value, ok
}

func TestV2ListMessagesUsesIndexedSummaries(t *testing.T) {
	router, st, account := newIndexedRouterWithStore(t)
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "msg-1",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          1,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "thread-1"),
		FromValue:    "Alice <alice@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Status Update",
		Snippet:      "Deployment completed successfully.",
		Seen:         false,
		Flagged:      true,
		Answered:     false,
		DateHeader:   time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "msg-2",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          2,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "thread-2"),
		FromValue:    "Bob <bob@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Roadmap",
		Snippet:      "Q2 planning notes.",
		Seen:         true,
		DateHeader:   time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
	})

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/messages?account_id="+url.QueryEscape(account.ID)+"&mailbox=INBOX&page=1&page_size=10", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []mail.MessageSummary `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Total != 2 || len(payload.Items) != 2 {
		t.Fatalf("expected 2 indexed summaries, got total=%d items=%d", payload.Total, len(payload.Items))
	}
	if payload.Items[0].Preview == "" || payload.Items[0].ThreadID == "" || payload.Items[0].Mailbox != "INBOX" {
		t.Fatalf("expected preview/thread/mailbox in summary, got %+v", payload.Items[0])
	}
	if payload.Items[0].AccountID != account.ID {
		t.Fatalf("expected summary account id %q, got %+v", account.ID, payload.Items[0])
	}
}

func TestV2ListMessagesWaitingFiltersLatestExternalThreads(t *testing.T) {
	router, st, account := newIndexedRouterWithStore(t)
	threadA := mail.ScopeIndexedThreadID(account.ID, "thread-a")
	threadB := mail.ScopeIndexedThreadID(account.ID, "thread-b")
	threadC := mail.ScopeIndexedThreadID(account.ID, "thread-c")

	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "wait-old",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          1,
		ThreadID:     threadA,
		FromValue:    "Alice <alice@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Release Plan",
		Snippet:      "First draft",
		Answered:     false,
		DateHeader:   time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "wait-latest",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          2,
		ThreadID:     threadA,
		FromValue:    "Alice <alice@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Re: Release Plan",
		Snippet:      "Following up",
		Answered:     false,
		DateHeader:   time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "self-latest",
		AccountID:    account.ID,
		Mailbox:      "Sent",
		UID:          3,
		ThreadID:     threadB,
		FromValue:    "account@example.com",
		ToValue:      "Bob <bob@example.com>",
		Subject:      "Sent follow-up",
		Snippet:      "Checking in",
		Answered:     false,
		DateHeader:   time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "already-answered",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          4,
		ThreadID:     threadC,
		FromValue:    "Carol <carol@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Status",
		Snippet:      "Did you get this?",
		Answered:     true,
		DateHeader:   time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
	})

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/messages?account_id="+url.QueryEscape(account.ID)+"&view=waiting&page=1&page_size=10", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []mail.MessageSummary `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode waiting payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Total != 1 || len(payload.Items) != 1 {
		t.Fatalf("expected one waiting thread, got total=%d items=%d body=%s", payload.Total, len(payload.Items), rec.Body.String())
	}
	if payload.Items[0].ID != "wait-latest" {
		t.Fatalf("expected latest external message for waiting view, got %+v", payload.Items[0])
	}
}

func TestV2SuggestRecipientsRanksSentRecipientsAndDedupes(t *testing.T) {
	router, st, account := newIndexedRouterWithStore(t)
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "sent-1",
		AccountID:    account.ID,
		Mailbox:      "Sent",
		UID:          1,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "sent-thread-1"),
		FromValue:    "account@example.com",
		ToValue:      "Alice Example <alice@example.com>",
		CCValue:      "Bob <bob@example.com>",
		Subject:      "Project",
		Snippet:      "Project update",
		DateHeader:   time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "sent-2",
		AccountID:    account.ID,
		Mailbox:      "Sent",
		UID:          2,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "sent-thread-2"),
		FromValue:    "account@example.com",
		ToValue:      "alice@example.com",
		Subject:      "Follow-up",
		Snippet:      "Checking back",
		DateHeader:   time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "inbound-1",
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          3,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "inbound-thread-1"),
		FromValue:    "Carol <carol@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Question",
		Snippet:      "Can you review this?",
		DateHeader:   time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
	})

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/recipients/suggest?account_id="+url.QueryEscape(account.ID)+"&q=&limit=5", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Items []models.RecipientSuggestion `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode recipient suggestions: %v body=%s", err, rec.Body.String())
	}
	if len(payload.Items) < 3 {
		t.Fatalf("expected at least three recipient suggestions, got %+v", payload.Items)
	}
	if payload.Items[0].Email != "alice@example.com" {
		t.Fatalf("expected most frequent sent recipient first, got %+v", payload.Items)
	}
	for _, item := range payload.Items {
		if item.Email == "account@example.com" {
			t.Fatalf("expected self address to be excluded, got %+v", payload.Items)
		}
	}
}

func TestV2ListAccountMailboxesIncludesCapabilities(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 2, Messages: 7},
			{Name: "Projects", Unread: 0, Messages: 0},
		},
	}
	withMailClientFactory(t, func(cfg config.Config) mail.Client { return client })
	router, _, account := newIndexedRouterWithStore(t)
	sessionCookie, csrfCookie := loginForSend(t, router)

	rec := authedV1Get(t, router, "/api/v2/accounts/"+url.PathEscape(account.ID)+"/mailboxes", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload []mail.Mailbox
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode mailbox payload: %v body=%s", err, rec.Body.String())
	}
	if len(payload) != 2 {
		t.Fatalf("expected 2 mailboxes, got %+v", payload)
	}
	if payload[0].CanRename || payload[0].CanDelete {
		t.Fatalf("expected inbox capabilities disabled, got %+v", payload[0])
	}
	if !payload[1].CanRename || !payload[1].CanDelete {
		t.Fatalf("expected custom folder capabilities enabled, got %+v", payload[1])
	}
}

func TestV2RenameAccountMailboxUpdatesIndexedState(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 1, Messages: 2},
			{Name: "Projects", Unread: 0, Messages: 0},
		},
	}
	withMailClientFactory(t, func(cfg config.Config) mail.Client { return client })
	router, st, account := newIndexedRouterWithStore(t)
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "project-msg",
		AccountID:    account.ID,
		Mailbox:      "Projects",
		UID:          7,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "project-thread"),
		FromValue:    "Alice <alice@example.com>",
		ToValue:      "account@example.com",
		Subject:      "Project update",
		Snippet:      "Project preview",
		DateHeader:   time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
	})
	if _, err := st.UpsertSyncState(context.Background(), store.SyncState{
		AccountID:   account.ID,
		Mailbox:     "Projects",
		UIDValidity: 1,
		UIDNext:     8,
	}); err != nil {
		t.Fatalf("seed sync state: %v", err)
	}

	sessionCookie, csrfCookie := loginForSend(t, router)
	rename := doV2AuthedJSON(t, router, http.MethodPatch, "/api/v2/accounts/"+account.ID+"/mailboxes", map[string]any{
		"mailbox_name":     "Projects",
		"new_mailbox_name": "Projects/2026",
	}, sessionCookie, csrfCookie)
	if rename.Code != http.StatusOK {
		t.Fatalf("expected rename 200, got %d body=%s", rename.Code, rename.Body.String())
	}
	if len(client.renamedMailboxes) != 1 || client.renamedMailboxes[0][0] != "Projects" || client.renamedMailboxes[0][1] != "Projects/2026" {
		t.Fatalf("expected rename call, got %+v", client.renamedMailboxes)
	}

	msg, err := st.GetIndexedMessageByID(context.Background(), account.ID, "project-msg")
	if err != nil {
		t.Fatalf("load renamed indexed message: %v", err)
	}
	if msg.Mailbox != "Projects/2026" {
		t.Fatalf("expected indexed mailbox updated, got %q", msg.Mailbox)
	}
	threads, total, err := st.ListThreads(context.Background(), account.ID, "", "", 20, 0)
	if err != nil {
		t.Fatalf("list threads after rename: %v", err)
	}
	if total != 1 || len(threads) != 1 || threads[0].Mailbox != "Projects/2026" {
		t.Fatalf("expected refreshed thread mailbox, got total=%d threads=%+v", total, threads)
	}
	if _, err := st.GetSyncState(context.Background(), account.ID, "Projects"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected old sync state removed, got err=%v", err)
	}
}

func TestV2DeleteAccountMailboxRejectsNonEmpty(t *testing.T) {
	client := &mailRouterTestClient{
		mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox", Unread: 1, Messages: 2},
			{Name: "Projects", Unread: 0, Messages: 3},
		},
	}
	withMailClientFactory(t, func(cfg config.Config) mail.Client { return client })
	router, _, account := newIndexedRouterWithStore(t)
	sessionCookie, csrfCookie := loginForSend(t, router)

	del := doV2AuthedJSON(t, router, http.MethodDelete, "/api/v2/accounts/"+account.ID+"/mailboxes", map[string]any{
		"mailbox_name": "Projects",
	}, sessionCookie, csrfCookie)
	if del.Code != http.StatusConflict {
		t.Fatalf("expected delete 409, got %d body=%s", del.Code, del.Body.String())
	}
	if !strings.Contains(del.Body.String(), "mailbox_not_empty") {
		t.Fatalf("expected mailbox_not_empty error, got body=%s", del.Body.String())
	}
}

func TestV2GetIndexedMessageAttachmentStreamsRawSource(t *testing.T) {
	router, st, account := newIndexedRouterWithStore(t)
	raw := "From: sender@example.com\r\n" +
		"To: account@example.com\r\n" +
		"Subject: Attachment\r\n" +
		"Message-ID: <attachment@example.com>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"mix\"\r\n" +
		"\r\n" +
		"--mix\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Body text.\r\n" +
		"--mix\r\n" +
		"Content-Type: text/plain\r\n" +
		"Content-Disposition: attachment; filename=\"report.txt\"\r\n" +
		"\r\n" +
		"hello attachment\r\n" +
		"--mix--\r\n"

	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:             "msg-attach",
		AccountID:      account.ID,
		Mailbox:        "INBOX",
		UID:            7,
		ThreadID:       mail.ScopeIndexedThreadID(account.ID, "thread-attach"),
		FromValue:      "Sender <sender@example.com>",
		ToValue:        "account@example.com",
		Subject:        "Attachment",
		Snippet:        "Body text.",
		BodyText:       "Body text.",
		RawSource:      raw,
		HasAttachments: true,
		DateHeader:     time.Date(2026, 3, 10, 9, 30, 0, 0, time.UTC),
		InternalDate:   time.Date(2026, 3, 10, 9, 30, 0, 0, time.UTC),
	})
	attachmentID := mail.EncodeAttachmentID(mail.NormalizeIndexedMessageID(account.ID, "msg-attach"), 1)
	if err := st.ReplaceIndexedAttachments(context.Background(), account.ID, "msg-attach", []models.IndexedAttachment{{
		ID:          attachmentID,
		MessageID:   mail.NormalizeIndexedMessageID(account.ID, "msg-attach"),
		AccountID:   account.ID,
		Filename:    "report.txt",
		ContentType: "text/plain",
		SizeBytes:   int64(len("hello attachment")),
	}}); err != nil {
		t.Fatalf("replace indexed attachments: %v", err)
	}

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/messages/msg-attach/attachments/"+url.PathEscape(attachmentID)+"?account_id="+url.QueryEscape(account.ID), sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != "hello attachment" {
		t.Fatalf("expected attachment body, got %q", body)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain" {
		t.Fatalf("expected text/plain content-type, got %q", got)
	}
}

func TestV2GetIndexedMessageRebuildsHTMLAndDecodesHeaders(t *testing.T) {
	router, st, account := newIndexedRouterWithStore(t)
	raw := strings.Join([]string{
		"From: =?utf-8?q?=D0=9C=D0=B5=D0=B4=D0=A2=D0=BE=D1=87=D0=BA=D0=B0?= <info@medtochka.ru>",
		"To: Admin <account@example.com>",
		"Subject: =?utf-8?b?0KLQtdGB0YLQvtCy0L7QtSDQv9C40YHRjNC80L4=?=",
		"MIME-Version: 1.0",
		"Content-Type: multipart/related; boundary=rel-1",
		"",
		"--rel-1",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<html><body><img src=\"cid:Logo.CID\"><img src=\"https://cdn.example.com/banner.png\"></body></html>",
		"--rel-1",
		"Content-Type: image/png",
		"Content-Transfer-Encoding: base64",
		"Content-Disposition: inline",
		"Content-ID: <Logo.CID>",
		"",
		"AQIDBA==",
		"--rel-1--",
		"",
	}, "\r\n")

	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:                "msg-html",
		AccountID:         account.ID,
		Mailbox:           "INBOX",
		UID:               11,
		ThreadID:          mail.ScopeIndexedThreadID(account.ID, "thread-html"),
		FromValue:         "=?utf-8?q?=D0=9C=D0=B5=D0=B4=D0=A2=D0=BE=D1=87=D0=BA=D0=B0?= <info@medtochka.ru>",
		ToValue:           "Admin <account@example.com>",
		Subject:           "=?utf-8?b?0KLQtdGB0YLQvtCy0L7QtSDQv9C40YHRjNC80L4=?=",
		Snippet:           "HTML body",
		BodyText:          "HTML body",
		BodyHTMLSanitized: "",
		RawSource:         raw,
		HasAttachments:    true,
		DateHeader:        time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		InternalDate:      time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
	})

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/messages/msg-html?account_id="+url.QueryEscape(account.ID), sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Message     models.IndexedMessage      `json:"message"`
		Attachments []models.IndexedAttachment `json:"attachments"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode message payload: %v body=%s", err, rec.Body.String())
	}
	if payload.Message.FromValue != "МедТочка <info@medtochka.ru>" {
		t.Fatalf("expected decoded sender, got %q", payload.Message.FromValue)
	}
	if payload.Message.Subject != "Тестовое письмо" {
		t.Fatalf("expected decoded subject, got %q", payload.Message.Subject)
	}
	if !strings.Contains(payload.Message.BodyHTMLSanitized, "/api/v2/messages/msg-html/attachments/") {
		t.Fatalf("expected cid image rewritten to v2 attachment endpoint, got %q", payload.Message.BodyHTMLSanitized)
	}
	if strings.Contains(payload.Message.BodyHTMLSanitized, "https://cdn.example.com/banner.png") {
		t.Fatalf("expected remote image URL to be proxied, got %q", payload.Message.BodyHTMLSanitized)
	}
	if !strings.Contains(payload.Message.BodyHTMLSanitized, "/api/v1/messages/") || !strings.Contains(payload.Message.BodyHTMLSanitized, "/remote-image?url=") {
		t.Fatalf("expected remote image proxy URL, got %q", payload.Message.BodyHTMLSanitized)
	}
}

func TestV2ListAggregateMailboxesMergesAccountsByRoleAndName(t *testing.T) {
	router, st, accountA := newIndexedRouterWithStore(t)
	ctx := context.Background()
	admin, err := st.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	accountB := createIndexedTestMailAccount(t, st, admin.ID, "backup@example.com", "Backup")

	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "msg-inbox-a",
		AccountID:    accountA.ID,
		Mailbox:      "INBOX",
		UID:          1,
		ThreadID:     mail.ScopeIndexedThreadID(accountA.ID, "thread-a"),
		FromValue:    "a@example.com",
		ToValue:      "admin@example.com",
		Subject:      "Inbox A",
		Snippet:      "Inbox A",
		DateHeader:   time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "msg-inbox-b",
		AccountID:    accountB.ID,
		Mailbox:      "INBOX",
		UID:          1,
		ThreadID:     mail.ScopeIndexedThreadID(accountB.ID, "thread-b"),
		FromValue:    "b@example.com",
		ToValue:      "admin@example.com",
		Subject:      "Inbox B",
		Snippet:      "Inbox B",
		DateHeader:   time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "msg-archive",
		AccountID:    accountA.ID,
		Mailbox:      "Archive",
		UID:          2,
		ThreadID:     mail.ScopeIndexedThreadID(accountA.ID, "thread-archive"),
		FromValue:    "archive@example.com",
		ToValue:      "admin@example.com",
		Subject:      "Archived",
		Snippet:      "Archived",
		DateHeader:   time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "msg-custom",
		AccountID:    accountB.ID,
		Mailbox:      "Projects/Alpha",
		UID:          3,
		ThreadID:     mail.ScopeIndexedThreadID(accountB.ID, "thread-custom"),
		FromValue:    "projects@example.com",
		ToValue:      "admin@example.com",
		Subject:      "Project",
		Snippet:      "Project",
		DateHeader:   time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
	})

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/mailboxes/aggregate", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload []mail.Mailbox
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode aggregate mailboxes: %v body=%s", err, rec.Body.String())
	}
	findMailbox := func(name string) *mail.Mailbox {
		t.Helper()
		for i := range payload {
			if strings.EqualFold(strings.TrimSpace(payload[i].Name), name) {
				return &payload[i]
			}
		}
		return nil
	}
	inbox := findMailbox("Inbox")
	if inbox == nil || inbox.Unread != 2 || inbox.Messages != 2 {
		t.Fatalf("expected merged inbox counts, got %+v", inbox)
	}
	archive := findMailbox("Archive")
	if archive == nil || archive.Messages != 1 {
		t.Fatalf("expected merged archive mailbox, got %+v", archive)
	}
	custom := findMailbox("Projects/Alpha")
	if custom == nil || custom.Messages != 1 {
		t.Fatalf("expected merged custom mailbox, got %+v", custom)
	}
}

func TestV2ListMessagesAllScopeIncludesAccountIDs(t *testing.T) {
	router, st, accountA := newIndexedRouterWithStore(t)
	ctx := context.Background()
	admin, err := st.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	accountB := createIndexedTestMailAccount(t, st, admin.ID, "backup@example.com", "Backup")

	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "msg-a",
		AccountID:    accountA.ID,
		Mailbox:      "INBOX",
		UID:          1,
		ThreadID:     mail.ScopeIndexedThreadID(accountA.ID, "thread-a"),
		FromValue:    "Alice <alice@example.com>",
		ToValue:      "admin@example.com",
		Subject:      "Primary",
		Snippet:      "Primary",
		DateHeader:   time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "msg-b",
		AccountID:    accountB.ID,
		Mailbox:      "INBOX",
		UID:          1,
		ThreadID:     mail.ScopeIndexedThreadID(accountB.ID, "thread-b"),
		FromValue:    "Bob <bob@example.com>",
		ToValue:      "admin@example.com",
		Subject:      "Backup",
		Snippet:      "Backup",
		DateHeader:   time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
	})

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/messages?account_scope=all&mailbox=Inbox&page=1&page_size=20", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []mail.MessageSummary `json:"items"`
		Total int                   `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode all-scope messages: %v body=%s", err, rec.Body.String())
	}
	if payload.Total != 2 || len(payload.Items) != 2 {
		t.Fatalf("expected 2 all-scope messages, got total=%d len=%d", payload.Total, len(payload.Items))
	}
	if payload.Items[0].AccountID != accountB.ID || payload.Items[1].AccountID != accountA.ID {
		t.Fatalf("expected newest account B then account A with account ids, got %+v", payload.Items)
	}
}

func TestV2SearchAllScopeIncludesAccountIDs(t *testing.T) {
	router, st, accountA := newIndexedRouterWithStore(t)
	ctx := context.Background()
	admin, err := st.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	accountB := createIndexedTestMailAccount(t, st, admin.ID, "backup@example.com", "Backup")

	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "search-a",
		AccountID:    accountA.ID,
		Mailbox:      "INBOX",
		UID:          1,
		ThreadID:     mail.ScopeIndexedThreadID(accountA.ID, "thread-a"),
		FromValue:    "Alice <alice@example.com>",
		ToValue:      "admin@example.com",
		Subject:      "Alpha rollout",
		Snippet:      "rollout notes",
		BodyText:     "alpha rollout notes",
		DateHeader:   time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
	})
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           "search-b",
		AccountID:    accountB.ID,
		Mailbox:      "INBOX",
		UID:          1,
		ThreadID:     mail.ScopeIndexedThreadID(accountB.ID, "thread-b"),
		FromValue:    "Bob <bob@example.com>",
		ToValue:      "admin@example.com",
		Subject:      "Alpha follow-up",
		Snippet:      "follow-up",
		BodyText:     "alpha follow-up",
		DateHeader:   time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 11, 0, 0, 0, time.UTC),
	})

	sessionCookie, csrfCookie := loginForSend(t, router)
	rec := authedV1Get(t, router, "/api/v2/search?account_scope=all&q=alpha&page=1&page_size=20", sessionCookie, csrfCookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items []models.IndexedMessage `json:"items"`
		Total int                     `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode all-scope search: %v body=%s", err, rec.Body.String())
	}
	if payload.Total != 2 || len(payload.Items) != 2 {
		t.Fatalf("expected 2 all-scope search results, got total=%d len=%d", payload.Total, len(payload.Items))
	}
	if payload.Items[0].AccountID != accountB.ID || payload.Items[1].AccountID != accountA.ID {
		t.Fatalf("expected account ids in search results, got %+v", payload.Items)
	}
}

func TestV2BulkMessagesUsesIMAPAndPatchesIndex(t *testing.T) {
	client := &indexedBulkTestMailClient{}
	previousFactory := mailClientFactory
	mailClientFactory = func(cfg config.Config) mail.Client { return client }
	t.Cleanup(func() {
		mailClientFactory = previousFactory
	})
	router, st := newV2RouterWithMailClientAndStore(t, client, nil)
	sess, csrf := loginV2(t, router)
	account := createV2TestAccount(t, router, sess, csrf, "indexed@example.com")

	legacyID := mail.EncodeMessageID("INBOX", 7)
	seedIndexedTestMessage(t, st, models.IndexedMessage{
		ID:           legacyID,
		AccountID:    account.ID,
		Mailbox:      "INBOX",
		UID:          7,
		ThreadID:     mail.ScopeIndexedThreadID(account.ID, "thread-bulk"),
		FromValue:    "Sender <sender@example.com>",
		ToValue:      "indexed@example.com",
		Subject:      "Bulk target",
		Snippet:      "bulk target",
		DateHeader:   time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC),
	})

	seen := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/messages/bulk", map[string]any{
		"account_id": account.ID,
		"ids":        []string{legacyID},
		"action":     "seen",
	}, sess, csrf)
	if seen.Code != http.StatusOK {
		t.Fatalf("expected seen bulk 200, got %d body=%s", seen.Code, seen.Body.String())
	}
	patch, ok := client.patchFor(legacyID)
	if !ok || len(patch.Add) != 1 || patch.Add[0] != "\\Seen" {
		t.Fatalf("expected IMAP seen flag update, got %+v ok=%v", patch, ok)
	}
	updated, err := st.GetIndexedMessageByID(context.Background(), account.ID, legacyID)
	if err != nil {
		t.Fatalf("load updated indexed message: %v", err)
	}
	if !updated.Seen {
		t.Fatalf("expected indexed message seen flag to be updated")
	}

	move := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/messages/bulk", map[string]any{
		"account_id": account.ID,
		"ids":        []string{legacyID},
		"action":     "move",
		"mailbox":    "Archive",
	}, sess, csrf)
	if move.Code != http.StatusOK {
		t.Fatalf("expected move bulk 200, got %d body=%s", move.Code, move.Body.String())
	}
	if target, ok := client.moveFor(legacyID); !ok || target != "Archive" {
		t.Fatalf("expected IMAP move to Archive, got target=%q ok=%v", target, ok)
	}
	moved, err := st.GetIndexedMessageByID(context.Background(), account.ID, legacyID)
	if err != nil {
		t.Fatalf("load moved indexed message: %v", err)
	}
	if moved.Mailbox != "Archive" {
		t.Fatalf("expected indexed mailbox updated to Archive, got %q", moved.Mailbox)
	}
}

func newIndexedRouterWithStore(t *testing.T) (http.Handler, *store.Store, models.MailAccount) {
	t.Helper()
	ctx := context.Background()
	sqdb, err := db.OpenSQLite(filepath.Join(t.TempDir(), "app.db"), 1, 1, time.Minute)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqdb.Close() })
	for _, migration := range []string{
		filepath.Join("..", "..", "migrations", "001_init.sql"),
		filepath.Join("..", "..", "migrations", "002_users_mail_login.sql"),
		filepath.Join("..", "..", "migrations", "003_cleanup_rejected_users.sql"),
		filepath.Join("..", "..", "migrations", "004_cleanup_rejected_users_casefold.sql"),
		filepath.Join("..", "..", "migrations", "005_admin_query_indexes.sql"),
		filepath.Join("..", "..", "migrations", "006_users_recovery_email.sql"),
		filepath.Join("..", "..", "migrations", "007_mail_accounts.sql"),
		filepath.Join("..", "..", "migrations", "008_mail_index.sql"),
		filepath.Join("..", "..", "migrations", "009_preferences_and_search.sql"),
		filepath.Join("..", "..", "migrations", "010_drafts_schedule.sql"),
		filepath.Join("..", "..", "migrations", "011_rules_sieve.sql"),
		filepath.Join("..", "..", "migrations", "012_mfa_totp_webauthn.sql"),
		filepath.Join("..", "..", "migrations", "013_crypto_keys.sql"),
		filepath.Join("..", "..", "migrations", "014_session_management.sql"),
		filepath.Join("..", "..", "migrations", "015_sync_state.sql"),
		filepath.Join("..", "..", "migrations", "016_quota_and_health.sql"),
		filepath.Join("..", "..", "migrations", "017_mfa_onboarding_flags.sql"),
		filepath.Join("..", "..", "migrations", "018_mfa_usability_trusted_devices.sql"),
		filepath.Join("..", "..", "migrations", "019_users_mail_secret.sql"),
		filepath.Join("..", "..", "migrations", "020_mail_index_scoped_ids.sql"),
		filepath.Join("..", "..", "migrations", "021_password_reset_token_reservations.sql"),
		filepath.Join("..", "..", "migrations", "022_draft_compose_context.sql"),
		filepath.Join("..", "..", "migrations", "023_drafts_nullable_account.sql"),
		filepath.Join("..", "..", "migrations", "024_draft_attachments_and_send_errors.sql"),
		filepath.Join("..", "..", "migrations", "025_session_mail_profiles.sql"),
		filepath.Join("..", "..", "migrations", "026_draft_context_account.sql"),
	} {
		if err := db.ApplyMigrationFile(sqdb, migration); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}

	st := store.New(sqdb)
	pwHash, err := auth.HashPassword("SecretPass123!")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := st.EnsureAdmin(ctx, "admin@example.com", pwHash); err != nil {
		t.Fatalf("ensure admin: %v", err)
	}
	admin, err := st.GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	if err := st.UpdateUserMailLogin(ctx, admin.ID, "account@example.com"); err != nil {
		t.Fatalf("set mail_login: %v", err)
	}
	secret, err := util.EncryptString(util.Derive32ByteKey(sendTestSessionEncryptKey), "mail-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	account, err := st.CreateMailAccount(ctx, models.MailAccount{
		UserID:       admin.ID,
		DisplayName:  "Primary",
		Login:        "account@example.com",
		SecretEnc:    secret,
		IMAPHost:     "imap.example.com",
		IMAPPort:     993,
		IMAPTLS:      true,
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPStartTLS: true,
		IsDefault:    true,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("create mail account: %v", err)
	}

	cfg := config.Config{
		ListenAddr:          ":8080",
		BaseDomain:          "example.com",
		SessionCookieName:   "despatch_session",
		CSRFCookieName:      "despatch_csrf",
		SessionIdleMinutes:  30,
		SessionAbsoluteHour: 24,
		SessionEncryptKey:   sendTestSessionEncryptKey,
		CookieSecureMode:    "never",
		TrustProxy:          false,
		PasswordMinLength:   12,
		PasswordMaxLength:   128,
		DovecotAuthMode:     "sql",
	}
	svc := service.New(cfg, st, mail.NoopClient{}, mail.NoopProvisioner{}, nil)
	return NewRouter(cfg, svc), st, account
}

func createIndexedTestMailAccount(t *testing.T, st *store.Store, userID, login, displayName string) models.MailAccount {
	t.Helper()
	secret, err := util.EncryptString(util.Derive32ByteKey(sendTestSessionEncryptKey), "mail-secret")
	if err != nil {
		t.Fatalf("encrypt account secret: %v", err)
	}
	account, err := st.CreateMailAccount(context.Background(), models.MailAccount{
		UserID:       userID,
		DisplayName:  displayName,
		Login:        login,
		SecretEnc:    secret,
		IMAPHost:     "imap.example.com",
		IMAPPort:     993,
		IMAPTLS:      true,
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPStartTLS: true,
		Status:       "active",
	})
	if err != nil {
		t.Fatalf("create indexed test account: %v", err)
	}
	return account
}

func seedIndexedTestMessage(t *testing.T, st *store.Store, item models.IndexedMessage) {
	t.Helper()
	item.ID = mail.NormalizeIndexedMessageID(item.AccountID, item.ID)
	item.ThreadID = mail.NormalizeIndexedThreadID(item.AccountID, item.ThreadID)
	if item.MessageIDHeader == "" {
		item.MessageIDHeader = item.ID + "@example.com"
	}
	if item.Subject == "" {
		item.Subject = "(no subject)"
	}
	if item.Snippet == "" {
		item.Snippet = "preview"
	}
	if item.BodyText == "" {
		item.BodyText = item.Snippet
	}
	if item.RawSource == "" {
		item.RawSource = item.BodyText
	}
	if item.DateHeader.IsZero() {
		item.DateHeader = time.Now().UTC()
	}
	if item.InternalDate.IsZero() {
		item.InternalDate = item.DateHeader
	}
	if _, err := st.UpsertIndexedMessage(context.Background(), item); err != nil {
		t.Fatalf("upsert indexed message: %v", err)
	}
}
