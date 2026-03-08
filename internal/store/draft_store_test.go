package store

import (
	"context"
	"testing"

	"despatch/internal/models"
)

func TestDraftCRUDSupportsComposeContextWithoutAccount(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()

	user, err := st.CreateUser(ctx, "drafts@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	created, err := st.CreateDraft(ctx, models.Draft{
		UserID:           user.ID,
		AccountID:        "",
		IdentityID:       "",
		ComposeMode:      "reply",
		ContextMessageID: "msg-123",
		FromMode:         "manual",
		FromManual:       "drafts@example.com",
		ClientStateJSON:  `{"cc_visible":true,"to_pending":"alice@example.com"}`,
		ToValue:          "alice@example.com",
		Subject:          "Draft subject",
		BodyText:         "Body text",
		BodyHTML:         "<p>Body text</p>",
		Status:           "active",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if created.AccountID != "" {
		t.Fatalf("expected blank account_id, got %q", created.AccountID)
	}
	if created.ComposeMode != "reply" || created.ContextMessageID != "msg-123" {
		t.Fatalf("unexpected compose context: %+v", created)
	}

	loaded, err := st.GetDraftByID(ctx, user.ID, created.ID)
	if err != nil {
		t.Fatalf("get draft: %v", err)
	}
	if loaded.FromMode != "manual" || loaded.FromManual != "drafts@example.com" {
		t.Fatalf("unexpected sender fields: %+v", loaded)
	}
	if loaded.ClientStateJSON == "" {
		t.Fatalf("expected client_state_json to persist")
	}

	loaded.Subject = "Updated subject"
	loaded.ComposeMode = "forward"
	loaded.ContextMessageID = "msg-456"
	loaded.ClientStateJSON = `{"bcc_visible":true}`
	updated, err := st.UpdateDraft(ctx, loaded)
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if updated.Subject != "Updated subject" || updated.ComposeMode != "forward" || updated.ContextMessageID != "msg-456" {
		t.Fatalf("unexpected updated draft: %+v", updated)
	}

	versions, err := st.ListDraftVersions(ctx, created.ID, 20)
	if err != nil {
		t.Fatalf("list draft versions: %v", err)
	}
	if len(versions) < 2 {
		t.Fatalf("expected at least 2 draft versions, got %d", len(versions))
	}

	if err := st.DeleteDraft(ctx, user.ID, created.ID); err != nil {
		t.Fatalf("delete draft: %v", err)
	}
	if _, err := st.GetDraftByID(ctx, user.ID, created.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestListDraftsExcludesSentDrafts(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()

	user, err := st.CreateUser(ctx, "drafts-list@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	active, err := st.CreateDraft(ctx, models.Draft{
		UserID:   user.ID,
		ToValue:  "alice@example.com",
		Subject:  "Keep me",
		BodyText: "hello",
		Status:   "active",
	})
	if err != nil {
		t.Fatalf("create active draft: %v", err)
	}
	sent, err := st.CreateDraft(ctx, models.Draft{
		UserID:   user.ID,
		ToValue:  "bob@example.com",
		Subject:  "Already sent",
		BodyText: "done",
		Status:   "active",
	})
	if err != nil {
		t.Fatalf("create sent draft: %v", err)
	}
	if err := st.SetDraftStatus(ctx, user.ID, sent.ID, "sent"); err != nil {
		t.Fatalf("set draft status: %v", err)
	}

	items, total, err := st.ListDrafts(ctx, user.ID, "", 50, 0)
	if err != nil {
		t.Fatalf("list drafts: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected only one active draft, got total=%d items=%d", total, len(items))
	}
	if items[0].ID != active.ID {
		t.Fatalf("expected active draft %q, got %q", active.ID, items[0].ID)
	}
}
