package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"despatch/internal/middleware"
	"despatch/internal/models"
	"despatch/internal/store"
	"despatch/internal/util"
)

type senderMutationRequest struct {
	Name          *string `json:"name"`
	DisplayName   *string `json:"display_name"`
	FromEmail     *string `json:"from_email"`
	ReplyTo       *string `json:"reply_to"`
	SignatureText *string `json:"signature_text"`
	SignatureHTML *string `json:"signature_html"`
	AccountID     *string `json:"account_id"`
	IsDefault     *bool   `json:"is_default"`
}

func senderRequestName(req senderMutationRequest) *string {
	if req.Name != nil {
		return req.Name
	}
	return req.DisplayName
}

func (h *Handlers) V2ListSenders(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.ListSenderProfiles(r.Context(), u)
	if err != nil {
		util.WriteError(w, 500, "mail_senders_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2CreateSender(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accountID := strings.TrimSpace(chi.URLParam(r, "id"))
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, accountID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "create_sender_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req senderMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	displayName := strings.TrimSpace(valueOrEmpty(senderRequestName(req)))
	fromEmail, err := normalizeRequiredMailboxAddress(valueOrEmpty(req.FromEmail), "from_email")
	if err != nil {
		util.WriteError(w, 400, "create_sender_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	replyTo, err := normalizeOptionalMailboxAddress(valueOrEmpty(req.ReplyTo), "reply_to")
	if err != nil {
		util.WriteError(w, 400, "create_sender_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	signatureHTML, signatureText := normalizeSignatureFields(valueOrEmpty(req.SignatureHTML), valueOrEmpty(req.SignatureText))
	saved, err := h.svc.Store().CreateMailIdentity(r.Context(), models.MailIdentity{
		AccountID:     accountID,
		DisplayName:   displayName,
		FromEmail:     fromEmail,
		ReplyTo:       replyTo,
		SignatureText: signatureText,
		SignatureHTML: signatureHTML,
		IsDefault:     req.IsDefault != nil && *req.IsDefault,
	})
	if err != nil {
		util.WriteError(w, 400, "create_sender_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if req.IsDefault != nil && *req.IsDefault {
		if err := h.svc.SetDefaultSenderProfile(r.Context(), u, saved.ID); err != nil {
			util.WriteError(w, 500, "create_sender_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
	}
	out, err := h.svc.GetSenderProfileByID(r.Context(), u, saved.ID)
	if err != nil {
		util.WriteError(w, 500, "create_sender_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2UpdateSender(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	senderID := strings.TrimSpace(chi.URLParam(r, "sender_id"))
	current, err := h.svc.GetSenderProfileByID(r.Context(), u, senderID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "sender_not_found", "sender not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req senderMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if current.IsPrimary {
		profile, err := h.svc.EnsureSessionMailProfile(r.Context(), u)
		if err != nil {
			util.WriteError(w, 500, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		if name := senderRequestName(req); name != nil {
			profile.DisplayName = strings.TrimSpace(*name)
		}
		if req.ReplyTo != nil {
			profile.ReplyTo, err = normalizeOptionalMailboxAddress(*req.ReplyTo, "reply_to")
			if err != nil {
				util.WriteError(w, 400, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
				return
			}
		}
		if req.SignatureHTML != nil {
			profile.SignatureHTML = strings.TrimSpace(*req.SignatureHTML)
		}
		if req.SignatureText != nil {
			profile.SignatureText = normalizeStoredSignatureText(*req.SignatureText)
		}
		profile.SignatureHTML, profile.SignatureText = normalizeSignatureFields(profile.SignatureHTML, profile.SignatureText)
		if _, err := h.svc.Store().UpsertSessionMailProfile(r.Context(), profile); err != nil {
			util.WriteError(w, 400, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		if req.IsDefault != nil && *req.IsDefault {
			if err := h.svc.SetDefaultSenderProfile(r.Context(), u, profile.ID); err != nil {
				util.WriteError(w, 500, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
				return
			}
		}
	} else {
		identity, err := h.svc.Store().GetMailIdentityByID(r.Context(), senderID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				util.WriteError(w, 404, "sender_not_found", "sender not found", middleware.RequestID(r.Context()))
				return
			}
			util.WriteError(w, 500, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, identity.AccountID); err != nil {
			util.WriteError(w, 403, "forbidden", "sender does not belong to current user", middleware.RequestID(r.Context()))
			return
		}
		if name := senderRequestName(req); name != nil {
			identity.DisplayName = strings.TrimSpace(*name)
		}
		if req.FromEmail != nil {
			identity.FromEmail, err = normalizeRequiredMailboxAddress(*req.FromEmail, "from_email")
			if err != nil {
				util.WriteError(w, 400, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
				return
			}
		}
		if req.ReplyTo != nil {
			identity.ReplyTo, err = normalizeOptionalMailboxAddress(*req.ReplyTo, "reply_to")
			if err != nil {
				util.WriteError(w, 400, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
				return
			}
		}
		if req.SignatureHTML != nil {
			identity.SignatureHTML = strings.TrimSpace(*req.SignatureHTML)
		}
		if req.SignatureText != nil {
			identity.SignatureText = normalizeStoredSignatureText(*req.SignatureText)
		}
		identity.SignatureHTML, identity.SignatureText = normalizeSignatureFields(identity.SignatureHTML, identity.SignatureText)
		if req.AccountID != nil {
			nextAccountID := strings.TrimSpace(*req.AccountID)
			if nextAccountID == "" {
				util.WriteError(w, 400, "update_sender_failed", "account_id is required", middleware.RequestID(r.Context()))
				return
			}
			if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, nextAccountID); err != nil {
				util.WriteError(w, 403, "forbidden", "account does not belong to current user", middleware.RequestID(r.Context()))
				return
			}
			identity.AccountID = nextAccountID
		}
		if req.IsDefault != nil {
			identity.IsDefault = *req.IsDefault
		}
		if _, err := h.svc.Store().UpdateMailIdentity(r.Context(), identity); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				util.WriteError(w, 404, "sender_not_found", "sender not found", middleware.RequestID(r.Context()))
				return
			}
			util.WriteError(w, 400, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		if req.IsDefault != nil && *req.IsDefault {
			if err := h.svc.SetDefaultSenderProfile(r.Context(), u, identity.ID); err != nil {
				util.WriteError(w, 500, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
				return
			}
		}
	}
	out, err := h.svc.GetSenderProfileByID(r.Context(), u, senderID)
	if err != nil {
		util.WriteError(w, 500, "update_sender_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2DeleteSender(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	senderID := strings.TrimSpace(chi.URLParam(r, "sender_id"))
	current, err := h.svc.GetSenderProfileByID(r.Context(), u, senderID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "sender_not_found", "sender not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "delete_sender_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if current.IsPrimary {
		util.WriteError(w, 400, "sender_protected", "primary sender cannot be deleted", middleware.RequestID(r.Context()))
		return
	}
	identity, err := h.svc.Store().GetMailIdentityByID(r.Context(), senderID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "sender_not_found", "sender not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "delete_sender_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if _, err := h.svc.Store().GetMailAccountByID(r.Context(), u.ID, identity.AccountID); err != nil {
		util.WriteError(w, 403, "forbidden", "sender does not belong to current user", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().DeleteMailIdentity(r.Context(), identity.AccountID, identity.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "sender_not_found", "sender not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "delete_sender_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	prefs, err := h.svc.Store().GetUserPreferences(r.Context(), u.ID)
	if err == nil && strings.TrimSpace(prefs.DefaultSenderID) == identity.ID {
		_ = h.svc.SetDefaultSenderProfile(r.Context(), u, "")
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func valueOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
