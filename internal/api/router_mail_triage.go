package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"despatch/internal/mail"
	"despatch/internal/middleware"
	"despatch/internal/models"
	"despatch/internal/store"
	"despatch/internal/util"
)

func normalizeAPIMailTriageSource(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "indexed":
		return "indexed"
	default:
		return "live"
	}
}

type mailTriageActionRequest struct {
	Targets        []models.MailTriageTarget `json:"targets"`
	SnoozedUntil   *time.Time                `json:"snoozed_until"`
	ClearSnooze    bool                      `json:"clear_snooze"`
	ReminderAt     *time.Time                `json:"reminder_at"`
	ClearReminder  bool                      `json:"clear_reminder"`
	CategoryID     string                    `json:"category_id"`
	CategoryName   string                    `json:"category_name"`
	ClearCategory  bool                      `json:"clear_category"`
	AddTagIDs      []string                  `json:"add_tag_ids"`
	AddTagNames    []string                  `json:"add_tag_names"`
	RemoveTagIDs   []string                  `json:"remove_tag_ids"`
	RemoveTagNames []string                  `json:"remove_tag_names"`
	ClearTags      bool                      `json:"clear_tags"`
}

func presentMailTriageTarget(item models.MailTriageTarget) models.MailTriageTarget {
	target := item
	target.Source = normalizeAPIMailTriageSource(target.Source)
	target.AccountID = strings.TrimSpace(target.AccountID)
	target.ThreadID = strings.TrimSpace(target.ThreadID)
	target.Mailbox = strings.TrimSpace(target.Mailbox)
	target.Subject = strings.TrimSpace(target.Subject)
	target.From = strings.TrimSpace(target.From)
	if target.Source == "indexed" {
		target.ThreadID = mail.UnscopeIndexedThreadID(target.ThreadID)
	}
	return target
}

func presentMailThreadTriageState(item models.MailThreadTriageState) models.MailThreadTriageState {
	item.Target = presentMailTriageTarget(item.Target)
	return item
}

func presentMailTriageReminder(item models.MailTriageReminder) models.MailTriageReminder {
	item.Source = normalizeAPIMailTriageSource(item.Source)
	item.AccountID = strings.TrimSpace(item.AccountID)
	item.ThreadID = strings.TrimSpace(item.ThreadID)
	item.Mailbox = strings.TrimSpace(item.Mailbox)
	item.Subject = strings.TrimSpace(item.Subject)
	item.From = strings.TrimSpace(item.From)
	if item.Source == "indexed" {
		item.ThreadID = mail.UnscopeIndexedThreadID(item.ThreadID)
	}
	return item
}

func (h *Handlers) V2ListMailTriageCatalog(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	categories, err := h.svc.Store().ListMailTriageCategories(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mail_triage_categories_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	tags, err := h.svc.Store().ListMailTriageTags(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mail_triage_tags_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{
		"categories": categories,
		"tags":       tags,
	})
}

func (h *Handlers) V2CreateMailTriageCategory(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	item, err := h.svc.Store().CreateMailTriageCategory(r.Context(), models.MailTriageCategory{
		UserID: u.ID,
		Name:   req.Name,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrConflict):
			util.WriteError(w, 409, "mail_triage_category_conflict", "category already exists", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "mail_triage_category_create_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 201, item)
}

func (h *Handlers) V2UpdateMailTriageCategory(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	item, err := h.svc.Store().UpdateMailTriageCategory(r.Context(), models.MailTriageCategory{
		ID:     strings.TrimSpace(chi.URLParam(r, "id")),
		UserID: u.ID,
		Name:   req.Name,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			util.WriteError(w, 404, "mail_triage_category_not_found", "category not found", middleware.RequestID(r.Context()))
		case errors.Is(err, store.ErrConflict):
			util.WriteError(w, 409, "mail_triage_category_conflict", "category already exists", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "mail_triage_category_update_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 200, item)
}

func (h *Handlers) V2DeleteMailTriageCategory(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.Store().DeleteMailTriageCategory(r.Context(), u.ID, strings.TrimSpace(chi.URLParam(r, "id"))); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "mail_triage_category_not_found", "category not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "mail_triage_category_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handlers) V2CreateMailTriageTag(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	item, err := h.svc.Store().CreateMailTriageTag(r.Context(), models.MailTriageTag{
		UserID: u.ID,
		Name:   req.Name,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrConflict):
			util.WriteError(w, 409, "mail_triage_tag_conflict", "tag already exists", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "mail_triage_tag_create_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 201, item)
}

func (h *Handlers) V2UpdateMailTriageTag(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	item, err := h.svc.Store().UpdateMailTriageTag(r.Context(), models.MailTriageTag{
		ID:     strings.TrimSpace(chi.URLParam(r, "id")),
		UserID: u.ID,
		Name:   req.Name,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			util.WriteError(w, 404, "mail_triage_tag_not_found", "tag not found", middleware.RequestID(r.Context()))
		case errors.Is(err, store.ErrConflict):
			util.WriteError(w, 409, "mail_triage_tag_conflict", "tag already exists", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "mail_triage_tag_update_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 200, item)
}

func (h *Handlers) V2DeleteMailTriageTag(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.Store().DeleteMailTriageTag(r.Context(), u.ID, strings.TrimSpace(chi.URLParam(r, "id"))); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "mail_triage_tag_not_found", "tag not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "mail_triage_tag_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handlers) V2ApplyMailTriage(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req mailTriageActionRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	states, err := h.svc.Store().ApplyMailThreadTriage(r.Context(), u.ID, req.Targets, models.MailTriageMutation{
		SnoozedUntil:   req.SnoozedUntil,
		ClearSnooze:    req.ClearSnooze,
		ReminderAt:     req.ReminderAt,
		ClearReminder:  req.ClearReminder,
		CategoryID:     req.CategoryID,
		CategoryName:   req.CategoryName,
		ClearCategory:  req.ClearCategory,
		AddTagIDs:      req.AddTagIDs,
		AddTagNames:    req.AddTagNames,
		RemoveTagIDs:   req.RemoveTagIDs,
		RemoveTagNames: req.RemoveTagNames,
		ClearTags:      req.ClearTags,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			util.WriteError(w, 404, "mail_triage_target_not_found", "triage target not found", middleware.RequestID(r.Context()))
		case errors.Is(err, store.ErrConflict):
			util.WriteError(w, 409, "mail_triage_conflict", "triage category or tag already exists", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "mail_triage_apply_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	for i := range states {
		states[i] = presentMailThreadTriageState(states[i])
	}
	util.WriteJSON(w, 200, map[string]any{
		"status": "ok",
		"items":  states,
	})
}

func (h *Handlers) V2PollDueMailTriageReminders(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().PollDueMailTriageReminders(r.Context(), u.ID, 24)
	if err != nil {
		util.WriteError(w, 500, "mail_triage_reminders_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	for i := range items {
		items[i] = presentMailTriageReminder(items[i])
	}
	util.WriteJSON(w, 200, map[string]any{
		"items": items,
	})
}
