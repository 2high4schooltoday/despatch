package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	stdmail "net/mail"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"despatch/internal/mail"
	"despatch/internal/middleware"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/util"
)

const (
	mailSnippetsSettingPrefix  = "mail.snippets."
	mailFavoritesSettingPrefix = "mail.favorites."
	undoSendGracePeriod        = 10 * time.Second
	mailActionHTTPTimeout      = 10 * time.Second
	mailSweepBatchLimit        = 500
)

type indexedMessageDetailEnvelope struct {
	models.IndexedMessage
	Unsubscribe *mail.UnsubscribeAction `json:"unsubscribe,omitempty"`
}

func mailSnippetsSettingKey(userID string) string {
	return mailSnippetsSettingPrefix + strings.TrimSpace(userID)
}

func mailFavoritesSettingKey(userID string) string {
	return mailFavoritesSettingPrefix + strings.TrimSpace(userID)
}

func normalizeMailSnippet(in models.MailSnippet) (models.MailSnippet, error) {
	in.ID = strings.TrimSpace(in.ID)
	in.Name = strings.TrimSpace(in.Name)
	in.Subject = strings.TrimSpace(in.Subject)
	in.Body = strings.ReplaceAll(in.Body, "\r\n", "\n")
	if in.Name == "" {
		return models.MailSnippet{}, fmt.Errorf("snippet name is required")
	}
	if strings.TrimSpace(in.Subject) == "" && strings.TrimSpace(in.Body) == "" {
		return models.MailSnippet{}, fmt.Errorf("snippet subject or body is required")
	}
	return in, nil
}

func normalizeMailFavorite(in models.MailFavorite) (models.MailFavorite, error) {
	in.ID = strings.TrimSpace(in.ID)
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	in.Label = strings.TrimSpace(in.Label)
	in.AccountID = strings.TrimSpace(in.AccountID)
	in.AccountScope = strings.ToLower(strings.TrimSpace(in.AccountScope))
	in.Mailbox = strings.TrimSpace(in.Mailbox)
	in.SmartView = strings.ToLower(strings.TrimSpace(in.SmartView))
	in.SavedSearchID = strings.TrimSpace(in.SavedSearchID)
	in.Sender = strings.ToLower(strings.TrimSpace(in.Sender))
	in.Domain = strings.ToLower(strings.TrimSpace(in.Domain))
	in.ThreadID = strings.TrimSpace(in.ThreadID)
	in.MessageID = strings.TrimSpace(in.MessageID)
	in.Subject = strings.TrimSpace(in.Subject)
	in.From = strings.TrimSpace(in.From)
	if in.AccountScope != "all" {
		in.AccountScope = "account"
	}
	switch in.Kind {
	case "mailbox":
		if in.Mailbox == "" {
			return models.MailFavorite{}, fmt.Errorf("mailbox favorite requires mailbox")
		}
		if in.Label == "" {
			in.Label = in.Mailbox
		}
	case "saved_view":
		if in.SavedSearchID == "" {
			return models.MailFavorite{}, fmt.Errorf("saved view favorite requires saved_search_id")
		}
		if in.Label == "" {
			in.Label = "Saved View"
		}
	case "smart_view":
		if in.SmartView == "" {
			return models.MailFavorite{}, fmt.Errorf("smart view favorite requires smart_view")
		}
		if in.Label == "" {
			in.Label = in.SmartView
		}
	case "sender":
		if in.Sender == "" {
			return models.MailFavorite{}, fmt.Errorf("sender favorite requires sender")
		}
		if in.Label == "" {
			in.Label = in.Sender
		}
	case "domain":
		if in.Domain == "" {
			return models.MailFavorite{}, fmt.Errorf("domain favorite requires domain")
		}
		if in.Label == "" {
			in.Label = in.Domain
		}
	case "thread":
		if in.ThreadID == "" {
			return models.MailFavorite{}, fmt.Errorf("thread favorite requires thread_id")
		}
		if in.Label == "" {
			in.Label = firstNonEmptyTrimmed(in.Subject, "Pinned conversation")
		}
	case "message":
		if in.MessageID == "" {
			return models.MailFavorite{}, fmt.Errorf("message favorite requires message_id")
		}
		if in.Label == "" {
			in.Label = firstNonEmptyTrimmed(in.Subject, "Pinned message")
		}
	default:
		return models.MailFavorite{}, fmt.Errorf("unsupported favorite kind")
	}
	return in, nil
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func messageSortTime(item models.IndexedMessage) time.Time {
	if !item.DateHeader.IsZero() {
		return item.DateHeader
	}
	return item.InternalDate
}

func mailFavoriteFingerprint(item models.MailFavorite) string {
	switch item.Kind {
	case "mailbox":
		return strings.Join([]string{item.Kind, item.AccountScope, strings.ToLower(item.AccountID), strings.ToLower(item.Mailbox)}, "|")
	case "saved_view":
		return strings.Join([]string{item.Kind, strings.ToLower(item.SavedSearchID)}, "|")
	case "smart_view":
		return strings.Join([]string{item.Kind, item.AccountScope, strings.ToLower(item.AccountID), item.SmartView}, "|")
	case "sender":
		return strings.Join([]string{item.Kind, item.AccountScope, strings.ToLower(item.AccountID), item.Sender}, "|")
	case "domain":
		return strings.Join([]string{item.Kind, item.AccountScope, strings.ToLower(item.AccountID), item.Domain}, "|")
	case "thread":
		return strings.Join([]string{item.Kind, strings.ToLower(item.AccountID), strings.ToLower(item.ThreadID)}, "|")
	case "message":
		return strings.Join([]string{item.Kind, strings.ToLower(item.AccountID), strings.ToLower(item.MessageID)}, "|")
	default:
		return strings.Join([]string{item.Kind, item.ID}, "|")
	}
}

func (h *Handlers) loadMailSnippets(ctx context.Context, userID string) ([]models.MailSnippet, error) {
	raw, ok, err := h.svc.Store().GetSetting(ctx, mailSnippetsSettingKey(userID))
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return []models.MailSnippet{}, nil
	}
	var items []models.MailSnippet
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	out := make([]models.MailSnippet, 0, len(items))
	for _, item := range items {
		normalized, normalizeErr := normalizeMailSnippet(item)
		if normalizeErr != nil {
			continue
		}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (h *Handlers) saveMailSnippets(ctx context.Context, userID string, items []models.MailSnippet) error {
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return h.svc.Store().UpsertSetting(ctx, mailSnippetsSettingKey(userID), string(payload))
}

func (h *Handlers) loadMailFavorites(ctx context.Context, userID string) ([]models.MailFavorite, error) {
	raw, ok, err := h.svc.Store().GetSetting(ctx, mailFavoritesSettingKey(userID))
	if err != nil {
		return nil, err
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return []models.MailFavorite{}, nil
	}
	var items []models.MailFavorite
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	out := make([]models.MailFavorite, 0, len(items))
	for _, item := range items {
		normalized, normalizeErr := normalizeMailFavorite(item)
		if normalizeErr != nil {
			continue
		}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (h *Handlers) saveMailFavorites(ctx context.Context, userID string, items []models.MailFavorite) error {
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return h.svc.Store().UpsertSetting(ctx, mailFavoritesSettingKey(userID), string(payload))
}

func (h *Handlers) V2ListMailSnippets(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.loadMailSnippets(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mail_snippets_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2CreateMailSnippet(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.MailSnippet
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = uuid.NewString()
	req.CreatedAt = time.Now().UTC()
	req.UpdatedAt = req.CreatedAt
	item, err := normalizeMailSnippet(req)
	if err != nil {
		util.WriteError(w, 400, "mail_snippet_invalid", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	items, err := h.loadMailSnippets(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mail_snippets_load_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	items = append(items, item)
	if err := h.saveMailSnippets(r.Context(), u.ID, items); err != nil {
		util.WriteError(w, 500, "mail_snippet_save_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, item)
}

func (h *Handlers) V2UpdateMailSnippet(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.MailSnippet
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = strings.TrimSpace(chi.URLParam(r, "id"))
	items, err := h.loadMailSnippets(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mail_snippets_load_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	found := false
	for i := range items {
		if items[i].ID != req.ID {
			continue
		}
		req.CreatedAt = items[i].CreatedAt
		req.UpdatedAt = time.Now().UTC()
		normalized, normalizeErr := normalizeMailSnippet(req)
		if normalizeErr != nil {
			util.WriteError(w, 400, "mail_snippet_invalid", normalizeErr.Error(), middleware.RequestID(r.Context()))
			return
		}
		items[i] = normalized
		req = normalized
		found = true
		break
	}
	if !found {
		util.WriteError(w, 404, "mail_snippet_not_found", "mail snippet not found", middleware.RequestID(r.Context()))
		return
	}
	if err := h.saveMailSnippets(r.Context(), u.ID, items); err != nil {
		util.WriteError(w, 500, "mail_snippet_save_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, req)
}

func (h *Handlers) V2DeleteMailSnippet(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	items, err := h.loadMailSnippets(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mail_snippets_load_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	next := make([]models.MailSnippet, 0, len(items))
	removed := false
	for _, item := range items {
		if item.ID == id {
			removed = true
			continue
		}
		next = append(next, item)
	}
	if !removed {
		util.WriteError(w, 404, "mail_snippet_not_found", "mail snippet not found", middleware.RequestID(r.Context()))
		return
	}
	if err := h.saveMailSnippets(r.Context(), u.ID, next); err != nil {
		util.WriteError(w, 500, "mail_snippet_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2ListMailFavorites(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.loadMailFavorites(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mail_favorites_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2CreateMailFavorite(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.MailFavorite
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.CreatedAt = time.Now().UTC()
	req.UpdatedAt = req.CreatedAt
	normalized, err := normalizeMailFavorite(req)
	if err != nil {
		util.WriteError(w, 400, "mail_favorite_invalid", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	items, err := h.loadMailFavorites(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mail_favorites_load_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	fingerprint := mailFavoriteFingerprint(normalized)
	for i := range items {
		if mailFavoriteFingerprint(items[i]) != fingerprint {
			continue
		}
		normalized.ID = items[i].ID
		normalized.CreatedAt = items[i].CreatedAt
		normalized.UpdatedAt = time.Now().UTC()
		items[i] = normalized
		if err := h.saveMailFavorites(r.Context(), u.ID, items); err != nil {
			util.WriteError(w, 500, "mail_favorite_save_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		util.WriteJSON(w, 200, normalized)
		return
	}
	normalized.ID = uuid.NewString()
	items = append(items, normalized)
	if err := h.saveMailFavorites(r.Context(), u.ID, items); err != nil {
		util.WriteError(w, 500, "mail_favorite_save_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, normalized)
}

func (h *Handlers) V2DeleteMailFavorite(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	items, err := h.loadMailFavorites(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "mail_favorites_load_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	next := make([]models.MailFavorite, 0, len(items))
	removed := false
	for _, item := range items {
		if item.ID == id {
			removed = true
			continue
		}
		next = append(next, item)
	}
	if !removed {
		util.WriteError(w, 404, "mail_favorite_not_found", "mail favorite not found", middleware.RequestID(r.Context()))
		return
	}
	if err := h.saveMailFavorites(r.Context(), u.ID, next); err != nil {
		util.WriteError(w, 500, "mail_favorite_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2UndoDraftSend(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	draftID := strings.TrimSpace(chi.URLParam(r, "id"))
	draft, err := h.svc.Store().GetDraftByID(r.Context(), u.ID, draftID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "draft_not_found", "draft not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "draft_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	item, err := h.svc.Store().GetScheduledSendByDraftID(r.Context(), u.ID, draftID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "undo_send_not_available", "undo send window has expired", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "undo_send_lookup_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if item.State != "queued" && item.State != "retrying" {
		util.WriteError(w, 409, "undo_send_not_available", "undo send window has expired", middleware.RequestID(r.Context()))
		return
	}
	if !item.DueAt.After(time.Now().UTC()) {
		util.WriteError(w, 409, "undo_send_not_available", "undo send window has expired", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().CancelScheduledSendByDraftID(r.Context(), u.ID, draftID); err != nil {
		util.WriteError(w, 500, "undo_send_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	draft.Status = "draft"
	draft.LastSendError = ""
	draft.ScheduledFor = time.Time{}
	draft.SendMode = "send_now"
	draft, err = h.svc.Store().UpdateDraft(r.Context(), draft)
	if err != nil {
		util.WriteError(w, 500, "undo_send_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{
		"status": "draft",
		"draft":  draft,
	})
}

func (h *Handlers) V1UnsubscribeMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		h.writeMailAuthError(w, r, err)
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	raw, err := h.svc.Mail().GetRawMessage(r.Context(), service.MailAuthLogin(u), pass, id)
	if err != nil {
		util.WriteError(w, 404, "message_not_found", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	action, err := mail.ParsePreferredUnsubscribeActionFromRaw(raw)
	if err != nil {
		util.WriteError(w, 422, "unsubscribe_not_available", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if action == nil {
		util.WriteError(w, 404, "unsubscribe_not_available", "unsubscribe action not available", middleware.RequestID(r.Context()))
		return
	}
	if err := h.performUnsubscribeAction(r.Context(), service.MailAuthLogin(u), pass, h.svc.Mail(), action); err != nil {
		util.WriteError(w, 502, "unsubscribe_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "unsubscribed", "method": action.Method})
}

func (h *Handlers) V2UnsubscribeIndexedMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	account, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID)
	if err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	msg, err := h.svc.Store().GetIndexedMessageByID(r.Context(), accountID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "message_not_found", "message not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "message_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	action, err := mail.ParsePreferredUnsubscribeActionFromRaw([]byte(msg.RawSource))
	if err != nil {
		util.WriteError(w, 422, "unsubscribe_not_available", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if action == nil {
		util.WriteError(w, 404, "unsubscribe_not_available", "unsubscribe action not available", middleware.RequestID(r.Context()))
		return
	}
	pass, err := h.accountMailSecret(account)
	if err != nil {
		util.WriteError(w, 500, "mail_secret_unavailable", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.performUnsubscribeAction(r.Context(), account.Login, pass, h.accountMailClient(account), action); err != nil {
		util.WriteError(w, 502, "unsubscribe_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "unsubscribed", "method": action.Method})
}

func (h *Handlers) performUnsubscribeAction(ctx context.Context, login, pass string, client mail.Client, action *mail.UnsubscribeAction) error {
	if action == nil {
		return fmt.Errorf("unsubscribe action is unavailable")
	}
	switch strings.ToUpper(strings.TrimSpace(action.Method)) {
	case "POST":
		return performUnsubscribeHTTP(ctx, action.URL, http.MethodPost)
	case "GET":
		return performUnsubscribeHTTP(ctx, action.URL, http.MethodGet)
	case "MAILTO":
		if client == nil {
			return fmt.Errorf("mail client unavailable")
		}
		return performUnsubscribeMailto(ctx, login, pass, client, action)
	default:
		return fmt.Errorf("unsupported unsubscribe method")
	}
}

func performUnsubscribeHTTP(ctx context.Context, targetURL, method string) error {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return fmt.Errorf("unsubscribe url is missing")
	}
	var body io.Reader
	if strings.EqualFold(method, http.MethodPost) {
		body = strings.NewReader("List-Unsubscribe=One-Click")
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Despatch/1.0")
	if strings.EqualFold(method, http.MethodPost) {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	client := &http.Client{Timeout: mailActionHTTPTimeout}
	if strings.EqualFold(method, http.MethodPost) {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unsubscribe endpoint returned %s", resp.Status)
	}
	return nil
}

func performUnsubscribeMailto(ctx context.Context, login, pass string, client mail.Client, action *mail.UnsubscribeAction) error {
	recipient := strings.TrimSpace(action.Email)
	if recipient == "" {
		return fmt.Errorf("unsubscribe email is missing")
	}
	subject := strings.TrimSpace(action.Subject)
	body := strings.TrimSpace(action.Body)
	if body == "" {
		body = "unsubscribe"
	}
	_, err := client.Send(ctx, login, pass, mail.SendRequest{
		To:      []string{recipient},
		Subject: subject,
		Body:    body,
	})
	return err
}

func (h *Handlers) V2SweepIndexedMessage(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(r.URL.Query().Get("account_id"))
	if accountID == "" {
		util.WriteError(w, 400, "bad_request", "account_id is required", middleware.RequestID(r.Context()))
		return
	}
	account, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID)
	if err != nil {
		util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		Action        string `json:"action"`
		TargetMailbox string `json:"target_mailbox"`
		TargetRole    string `json:"target_role"`
		ApplyToFuture bool   `json:"apply_to_future"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	messageID := strings.TrimSpace(chi.URLParam(r, "id"))
	msg, err := h.svc.Store().GetIndexedMessageByID(r.Context(), accountID, messageID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "message_not_found", "message not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "message_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	senderEmail := indexedPrimaryEmail(msg.FromValue)
	if senderEmail == "" {
		util.WriteError(w, 422, "sweep_sender_missing", "sender address is unavailable for this message", middleware.RequestID(r.Context()))
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	targetRole := strings.ToLower(strings.TrimSpace(req.TargetRole))
	targetMailbox := strings.TrimSpace(req.TargetMailbox)
	switch action {
	case "archive", "trash", "move", "keep_latest":
	default:
		util.WriteError(w, 400, "bad_request", "unsupported sweep action", middleware.RequestID(r.Context()))
		return
	}
	pass, err := h.accountMailSecret(account)
	if err != nil {
		util.WriteError(w, 500, "mail_secret_unavailable", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	client := h.accountMailClient(account)
	if action == "archive" {
		targetRole = "archive"
	}
	if action == "trash" {
		targetRole = "trash"
	}
	if action == "keep_latest" {
		targetRole = "archive"
	}
	resolvedTarget, err := h.resolveSweepTargetMailbox(r.Context(), account, pass, client, targetMailbox, targetRole)
	if err != nil {
		writeMailboxMutationIMAPError(w, r, err)
		return
	}
	items, _, err := h.svc.Store().ListIndexedMessages(r.Context(), accountID, "", models.IndexedMessageFilter{From: senderEmail}, "", mailSweepBatchLimit, 0)
	if err != nil {
		util.WriteError(w, 500, "sweep_load_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	matches := make([]models.IndexedMessage, 0, len(items))
	for _, item := range items {
		if indexedPrimaryEmail(item.FromValue) == senderEmail {
			matches = append(matches, item)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		return messageSortTime(matches[i]).After(messageSortTime(matches[j]))
	})
	if action == "keep_latest" {
		if len(matches) > 0 {
			matches = matches[1:]
		} else {
			matches = matches[:0]
		}
	}
	applied := make([]string, 0, len(matches))
	failed := make([]map[string]string, 0)
	for _, item := range matches {
		rawID := mail.UnscopeIndexedMessageID(item.ID)
		if err := client.Move(r.Context(), account.Login, pass, rawID, resolvedTarget); err != nil {
			failed = append(failed, map[string]string{
				"id":      rawID,
				"message": err.Error(),
			})
			continue
		}
		if err := h.svc.Store().MoveIndexedMessageMailbox(r.Context(), accountID, item.ID, resolvedTarget); err != nil && !errors.Is(err, store.ErrNotFound) {
			failed = append(failed, map[string]string{
				"id":      rawID,
				"message": err.Error(),
			})
			continue
		}
		applied = append(applied, rawID)
	}
	ruleCreated := false
	ruleLive := false
	ruleID := ""
	if req.ApplyToFuture && action != "keep_latest" {
		if createdID, live, createErr := h.createSweepRule(r.Context(), account, pass, client, senderEmail, resolvedTarget, targetRole); createErr == nil {
			ruleCreated = true
			ruleLive = live
			ruleID = createdID
		}
	}
	util.WriteJSON(w, 200, map[string]any{
		"status":              "applied",
		"matched":             len(matches),
		"applied":             applied,
		"failed":              failed,
		"sender":              senderEmail,
		"target_mailbox":      resolvedTarget,
		"future_rule_created": ruleCreated,
		"future_rule_live":    ruleLive,
		"rule_id":             ruleID,
	})
}

func (h *Handlers) resolveSweepTargetMailbox(ctx context.Context, account models.MailAccount, pass string, client mail.Client, targetMailbox, targetRole string) (string, error) {
	targetMailbox = strings.TrimSpace(targetMailbox)
	targetRole = strings.ToLower(strings.TrimSpace(targetRole))
	if targetMailbox != "" {
		return targetMailbox, nil
	}
	if targetRole == "" {
		return "", fmt.Errorf("target mailbox is required")
	}
	resolved, err := h.resolveAccountSpecialMailboxByRole(ctx, account, pass, targetRole, client)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(resolved) == "" {
		return "", fmt.Errorf("special mailbox for %s is unavailable", targetRole)
	}
	return strings.TrimSpace(resolved), nil
}

func (h *Handlers) createSweepRule(ctx context.Context, account models.MailAccount, pass string, client mail.Client, senderEmail, targetMailbox, targetRole string) (string, bool, error) {
	existing, err := h.svc.Store().ListMailRules(ctx, account.ID)
	if err != nil {
		return "", false, err
	}
	for _, rule := range existing {
		if strings.ToLower(strings.TrimSpace(rule.Conditions.FromContains)) != strings.ToLower(strings.TrimSpace(senderEmail)) {
			continue
		}
		if strings.ToLower(strings.TrimSpace(rule.Actions.MoveToMailbox)) == strings.ToLower(strings.TrimSpace(targetMailbox)) ||
			strings.ToLower(strings.TrimSpace(rule.Actions.MoveToRole)) == strings.ToLower(strings.TrimSpace(targetRole)) {
			live, liveErr := h.managedRulesActive(ctx, account.ID)
			return rule.ID, liveErr == nil && live, nil
		}
	}
	rule := models.MailRule{
		ID:        uuid.NewString(),
		AccountID: account.ID,
		Name:      fmt.Sprintf("Sweep %s", senderEmail),
		Enabled:   true,
		Position:  len(existing),
		MatchMode: "all",
		Conditions: models.MailRuleConditions{
			FromContains: senderEmail,
		},
		Actions: models.MailRuleActions{
			MoveToMailbox: targetMailbox,
			Stop:          true,
		},
	}
	if targetRole == "trash" {
		rule.Actions.MoveToMailbox = ""
		rule.Actions.MoveToRole = "trash"
	}
	created, err := h.svc.Store().CreateMailRule(ctx, rule)
	if err != nil {
		return "", false, err
	}
	mailboxes, err := h.rawAccountMailboxes(ctx, account, pass)
	if err != nil {
		return created.ID, false, err
	}
	if err := h.syncManagedRulesScript(ctx, account, mailboxes); err != nil {
		return created.ID, false, err
	}
	live, err := h.managedRulesActive(ctx, account.ID)
	if err != nil {
		return created.ID, false, nil
	}
	return created.ID, live, nil
}

func (h *Handlers) managedRulesActive(ctx context.Context, accountID string) (bool, error) {
	scripts, err := h.svc.Store().ListSieveScripts(ctx, accountID)
	if err != nil {
		return false, err
	}
	for _, item := range scripts {
		if item.IsActive && strings.TrimSpace(item.ScriptName) == managedRulesScriptName {
			return true, nil
		}
	}
	return false, nil
}

func messageUnsubscribeAction(raw []byte) *mail.UnsubscribeAction {
	action, err := mail.ParsePreferredUnsubscribeActionFromRaw(raw)
	if err != nil {
		return nil
	}
	return action
}

func messagePrimaryDomain(raw string) string {
	email := indexedPrimaryEmail(raw)
	if email == "" {
		return ""
	}
	if parsed, err := stdmail.ParseAddress(email); err == nil {
		email = strings.TrimSpace(parsed.Address)
	}
	if at := strings.LastIndex(email, "@"); at >= 0 && at < len(email)-1 {
		return strings.ToLower(strings.TrimSpace(email[at+1:]))
	}
	return ""
}
