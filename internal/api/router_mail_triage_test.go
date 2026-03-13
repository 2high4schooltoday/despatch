package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"despatch/internal/models"
)

func TestV2MailTriageActionsCatalogCRUDAndDueReminderPolling(t *testing.T) {
	router, _, _ := newSendRouterWithStore(t, &mailRouterTestClient{}, "")
	sess, csrf := loginForSend(t, router)

	reminderAt := time.Now().Add(-90 * time.Second).UTC().Truncate(time.Second)
	actionRec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mail-triage/actions", map[string]any{
		"targets": []map[string]any{{
			"source":    "live",
			"thread_id": "thread-1",
			"mailbox":   "INBOX",
			"subject":   "Budget review",
			"from":      "alice@example.com",
		}},
		"category_name": "Finance",
		"add_tag_names": []string{"urgent", "billing"},
		"reminder_at":   reminderAt.Format(time.RFC3339Nano),
	}, sess, csrf)
	if actionRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from triage action, got %d body=%s", actionRec.Code, actionRec.Body.String())
	}

	var actionPayload struct {
		Status string                         `json:"status"`
		Items  []models.MailThreadTriageState `json:"items"`
	}
	if err := json.Unmarshal(actionRec.Body.Bytes(), &actionPayload); err != nil {
		t.Fatalf("decode triage action: %v", err)
	}
	if actionPayload.Status != "ok" || len(actionPayload.Items) != 1 {
		t.Fatalf("unexpected triage action payload: %#v", actionPayload)
	}
	if actionPayload.Items[0].Target.Source != "live" || actionPayload.Items[0].Target.ThreadID != "thread-1" {
		t.Fatalf("unexpected triage target: %#v", actionPayload.Items[0].Target)
	}
	if actionPayload.Items[0].Triage.Category == nil || actionPayload.Items[0].Triage.Category.Name != "Finance" {
		t.Fatalf("expected Finance category, got %#v", actionPayload.Items[0].Triage.Category)
	}
	if len(actionPayload.Items[0].Triage.Tags) != 2 {
		t.Fatalf("expected 2 triage tags, got %#v", actionPayload.Items[0].Triage.Tags)
	}
	if actionPayload.Items[0].Triage.ReminderAt == nil || !actionPayload.Items[0].Triage.ReminderAt.UTC().Equal(reminderAt) {
		t.Fatalf("unexpected reminder timestamp: %#v", actionPayload.Items[0].Triage.ReminderAt)
	}

	catalogRec := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/mail-triage/catalog", nil, sess, csrf)
	if catalogRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from triage catalog, got %d body=%s", catalogRec.Code, catalogRec.Body.String())
	}
	var catalog struct {
		Categories []models.MailTriageCategory `json:"categories"`
		Tags       []models.MailTriageTag      `json:"tags"`
	}
	if err := json.Unmarshal(catalogRec.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("decode triage catalog: %v", err)
	}
	if len(catalog.Categories) != 1 || catalog.Categories[0].Name != "Finance" {
		t.Fatalf("unexpected triage categories: %#v", catalog.Categories)
	}
	if len(catalog.Tags) != 2 {
		t.Fatalf("unexpected triage tags: %#v", catalog.Tags)
	}

	createCategoryRec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mail-triage/categories", map[string]any{
		"name": "Ops",
	}, sess, csrf)
	if createCategoryRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating category, got %d body=%s", createCategoryRec.Code, createCategoryRec.Body.String())
	}
	var createdCategory models.MailTriageCategory
	if err := json.Unmarshal(createCategoryRec.Body.Bytes(), &createdCategory); err != nil {
		t.Fatalf("decode created category: %v", err)
	}

	updateCategoryRec := doV2AuthedJSON(t, router, http.MethodPatch, "/api/v2/mail-triage/categories/"+createdCategory.ID, map[string]any{
		"name": "Operations",
	}, sess, csrf)
	if updateCategoryRec.Code != http.StatusOK {
		t.Fatalf("expected 200 updating category, got %d body=%s", updateCategoryRec.Code, updateCategoryRec.Body.String())
	}

	createTagRec := doV2AuthedJSON(t, router, http.MethodPost, "/api/v2/mail-triage/tags", map[string]any{
		"name": "waiting",
	}, sess, csrf)
	if createTagRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating tag, got %d body=%s", createTagRec.Code, createTagRec.Body.String())
	}
	var createdTag models.MailTriageTag
	if err := json.Unmarshal(createTagRec.Body.Bytes(), &createdTag); err != nil {
		t.Fatalf("decode created tag: %v", err)
	}

	deleteTagRec := doV2AuthedJSON(t, router, http.MethodDelete, "/api/v2/mail-triage/tags/"+createdTag.ID, nil, sess, csrf)
	if deleteTagRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting tag, got %d body=%s", deleteTagRec.Code, deleteTagRec.Body.String())
	}

	reminderRec := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/mail-triage/reminders/due", nil, sess, csrf)
	if reminderRec.Code != http.StatusOK {
		t.Fatalf("expected 200 polling reminders, got %d body=%s", reminderRec.Code, reminderRec.Body.String())
	}
	var reminders struct {
		Items []models.MailTriageReminder `json:"items"`
	}
	if err := json.Unmarshal(reminderRec.Body.Bytes(), &reminders); err != nil {
		t.Fatalf("decode due reminders: %v", err)
	}
	if len(reminders.Items) != 1 {
		t.Fatalf("expected 1 due reminder, got %#v", reminders.Items)
	}
	if reminders.Items[0].Source != "live" || reminders.Items[0].ThreadID != "thread-1" {
		t.Fatalf("unexpected reminder payload: %#v", reminders.Items[0])
	}

	reminderRec2 := doV2AuthedJSON(t, router, http.MethodGet, "/api/v2/mail-triage/reminders/due", nil, sess, csrf)
	if reminderRec2.Code != http.StatusOK {
		t.Fatalf("expected 200 polling reminders second time, got %d body=%s", reminderRec2.Code, reminderRec2.Body.String())
	}
	var reminders2 struct {
		Items []models.MailTriageReminder `json:"items"`
	}
	if err := json.Unmarshal(reminderRec2.Body.Bytes(), &reminders2); err != nil {
		t.Fatalf("decode due reminders second poll: %v", err)
	}
	if len(reminders2.Items) != 0 {
		t.Fatalf("expected due reminders to dedupe after first poll, got %#v", reminders2.Items)
	}
}
