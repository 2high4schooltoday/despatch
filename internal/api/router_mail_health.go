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
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/util"
)

const mailHealthAttentionThreshold = 10 * time.Minute

func mailAccountHealthLabel(account models.MailAccount) string {
	if label := strings.TrimSpace(account.DisplayName); label != "" {
		return label
	}
	if label := strings.TrimSpace(account.Login); label != "" {
		return label
	}
	return strings.TrimSpace(account.ID)
}

func classifyMailAccountHealth(now time.Time, account models.MailAccount) string {
	if strings.TrimSpace(account.LastError) != "" {
		return "error"
	}
	if account.LastSyncAt.IsZero() {
		return "attention"
	}
	if now.Sub(account.LastSyncAt.UTC()) > mailHealthAttentionThreshold {
		return "attention"
	}
	return "ok"
}

func quotaHealthFromCache(item models.QuotaCache) (available, supported bool, lastErr string) {
	errText := strings.TrimSpace(item.LastError)
	if errText == "" {
		return true, true, ""
	}
	if errText == mail.ErrQuotaUnsupported.Error() {
		return false, false, "Quota unavailable on this server."
	}
	hasValues := item.UsedBytes > 0 || item.TotalBytes > 0 || item.UsedMessages > 0 || item.TotalMessages > 0
	return hasValues, true, errText
}

func (h *Handlers) V2ListAccountHealth(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.EnsureAuthenticatedMailAccount(r.Context(), u); err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	accounts, err := h.svc.Store().ListMailAccounts(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	summary := models.MailAccountHealthSummary{}
	items := make([]models.MailAccountHealth, 0, len(accounts))
	now := time.Now().UTC()
	coordinator := h.svc.MailHealthCoordinator()
	for _, account := range accounts {
		item := models.MailAccountHealth{
			AccountID:      account.ID,
			AccountLabel:   mailAccountHealthLabel(account),
			IsDefault:      account.IsDefault,
			Status:         classifyMailAccountHealth(now, account),
			LastSyncAt:     account.LastSyncAt,
			LastError:      strings.TrimSpace(account.LastError),
			QuotaSupported: true,
		}
		switch item.Status {
		case "ok":
			summary.HealthyAccounts++
		case "error":
			summary.ErrorAccounts++
		default:
			summary.AttentionAccounts++
		}
		summary.TotalAccounts++
		cache, cacheErr := h.svc.Store().GetQuotaCacheByAccount(r.Context(), account.ID)
		if cacheErr != nil {
			if !errors.Is(cacheErr, store.ErrNotFound) {
				util.WriteError(w, 500, "quota_get_failed", cacheErr.Error(), middleware.RequestID(r.Context()))
				return
			}
		} else {
			item.UsedBytes = cache.UsedBytes
			item.TotalBytes = cache.TotalBytes
			item.UsedMessages = cache.UsedMessages
			item.TotalMessages = cache.TotalMessages
			item.QuotaRefreshedAt = cache.RefreshedAt
			item.QuotaAvailable, item.QuotaSupported, item.QuotaLastError = quotaHealthFromCache(cache)
		}
		if coordinator != nil {
			if actionState, ok := coordinator.ActionState(account.ID); ok {
				copyState := actionState
				item.ActionState = &copyState
			}
		}
		items = append(items, item)
	}
	util.WriteJSON(w, 200, map[string]any{
		"summary": summary,
		"items":   items,
	})
}

func (h *Handlers) queueMailHealthAction(w http.ResponseWriter, r *http.Request, kind string) {
	u, _ := middleware.User(r.Context())
	coordinator := h.svc.MailHealthCoordinator()
	if coordinator == nil {
		util.WriteError(w, 503, "mail_health_unavailable", "mail health actions are unavailable", middleware.RequestID(r.Context()))
		return
	}
	accountID := strings.TrimSpace(chi.URLParam(r, "id"))
	account, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "internal_error", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var state models.MailHealthActionState
	switch kind {
	case "sync":
		state, err = coordinator.QueueAccountSync(r.Context(), account)
	case "quota_refresh":
		state, err = coordinator.QueueQuotaRefresh(r.Context(), account)
	case "reindex":
		state, err = coordinator.QueueAccountReindex(r.Context(), account)
	default:
		util.WriteError(w, 400, "bad_request", "unsupported health action", middleware.RequestID(r.Context()))
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMailHealthActionInProgress):
			util.WriteError(w, 409, "health_action_in_progress", "an account health action is already in progress", middleware.RequestID(r.Context()))
		case errors.Is(err, service.ErrMailHealthUnavailable):
			util.WriteError(w, 503, "mail_health_unavailable", "mail health actions are unavailable", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "mail_health_action_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status":       "queued",
		"account_id":   account.ID,
		"action_state": state,
	})
}

func (h *Handlers) V2QueueAccountHealthSync(w http.ResponseWriter, r *http.Request) {
	h.queueMailHealthAction(w, r, "sync")
}

func (h *Handlers) V2QueueAccountQuotaRefresh(w http.ResponseWriter, r *http.Request) {
	h.queueMailHealthAction(w, r, "quota_refresh")
}

func (h *Handlers) V2QueueAccountReindex(w http.ResponseWriter, r *http.Request) {
	h.queueMailHealthAction(w, r, "reindex")
}
