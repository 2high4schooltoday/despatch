package api

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"despatch/internal/mail"
	"despatch/internal/middleware"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/util"
)

type specialMailboxMappingDTO struct {
	Role        string `json:"role"`
	MailboxName string `json:"mailbox_name"`
}

var specialMailboxRoles = []string{"sent", "archive", "trash"}

func specialMailboxMappingsForResponse(mappings map[string]string) []specialMailboxMappingDTO {
	out := make([]specialMailboxMappingDTO, 0, len(mappings))
	for _, role := range specialMailboxRoles {
		name := strings.TrimSpace(mappings[role])
		if name == "" {
			continue
		}
		out = append(out, specialMailboxMappingDTO{
			Role:        role,
			MailboxName: name,
		})
	}
	return out
}

func applySpecialMailboxRoles(mailboxes []mail.Mailbox, mappings map[string]string) []mail.Mailbox {
	out := make([]mail.Mailbox, len(mailboxes))
	copy(out, mailboxes)
	for _, role := range specialMailboxRoles {
		target := strings.TrimSpace(mappings[role])
		if target == "" {
			continue
		}
		for i := range out {
			currentRole := strings.ToLower(strings.TrimSpace(out[i].Role))
			if currentRole == role && !strings.EqualFold(strings.TrimSpace(out[i].Name), target) {
				out[i].Role = ""
			}
		}
		for i := range out {
			if strings.EqualFold(strings.TrimSpace(out[i].Name), target) {
				out[i].Role = role
				break
			}
		}
	}
	return out
}

func resolveSpecialMailboxFromAvailable(mailboxes []mail.Mailbox, mappings map[string]string, role string) string {
	normalizedRole := strings.ToLower(strings.TrimSpace(role))
	if normalizedRole == "" {
		return ""
	}
	return strings.TrimSpace(mail.ResolveMailboxByRole(applySpecialMailboxRoles(mailboxes, mappings), normalizedRole))
}

func (h *Handlers) listMailboxesWithSpecialRoles(r *http.Request) ([]mail.Mailbox, map[string]string, string, string, error) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		return nil, nil, "", "", err
	}
	mailLogin := service.MailIdentity(u)
	items, err := h.rawMailboxes(r.Context(), mailLogin, pass)
	if err != nil {
		return nil, nil, mailLogin, pass, err
	}
	mappings, err := h.svc.Store().ListSpecialMailboxMappings(r.Context(), u.ID, mailLogin)
	if err != nil {
		return nil, nil, mailLogin, pass, err
	}
	return applySpecialMailboxRoles(items, mappings), mappings, mailLogin, pass, nil
}

func (h *Handlers) resolveSessionSpecialMailboxByRole(ctx context.Context, u models.User, pass, role string) (string, error) {
	mailLogin := service.MailIdentity(u)
	items, err := h.rawMailboxes(ctx, mailLogin, pass)
	if err != nil {
		return "", err
	}
	mappings, err := h.svc.Store().ListSpecialMailboxMappings(ctx, u.ID, mailLogin)
	if err != nil {
		return "", err
	}
	return resolveSpecialMailboxFromAvailable(items, mappings, role), nil
}

func resolveMappedMailboxByRole(items []models.MailboxMapping, role string) string {
	target := strings.ToLower(strings.TrimSpace(role))
	if target == "" {
		return ""
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Role), target) {
			return strings.TrimSpace(item.MailboxName)
		}
	}
	return ""
}

func (h *Handlers) resolveAccountSpecialMailboxByRole(ctx context.Context, account models.MailAccount, pass, role string, client mail.Client) (string, error) {
	items, err := client.ListMailboxes(ctx, account.Login, pass)
	if err != nil {
		return "", err
	}
	mappings, err := h.svc.Store().ListMailboxMappings(ctx, account.ID)
	if err != nil {
		return "", err
	}
	overlay := map[string]string{}
	if mapped := resolveMappedMailboxByRole(mappings, role); mapped != "" {
		overlay[strings.ToLower(strings.TrimSpace(role))] = mapped
	}
	return resolveSpecialMailboxFromAvailable(items, overlay, role), nil
}

func isSessionMailAuthError(err error) bool {
	if err == nil {
		return false
	}
	return err == service.ErrInvalidCredentials || strings.Contains(err.Error(), "mail credentials")
}

func (h *Handlers) ListSpecialMailboxes(w http.ResponseWriter, r *http.Request) {
	_, mappings, _, _, err := h.listMailboxesWithSpecialRoles(r)
	if err != nil {
		if isSessionMailAuthError(err) {
			h.writeMailAuthError(w, r, err)
			return
		}
		util.WriteError(w, 500, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{
		"items": specialMailboxMappingsForResponse(mappings),
	})
}

func (h *Handlers) UpsertSpecialMailbox(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	role := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "role")))
	if role != "sent" && role != "archive" && role != "trash" {
		util.WriteError(w, 400, "bad_request", "unsupported mailbox role", middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		MailboxName     string `json:"mailbox_name"`
		CreateIfMissing bool   `json:"create_if_missing"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	target := strings.TrimSpace(req.MailboxName)
	if target == "" {
		util.WriteError(w, 400, "bad_request", "mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}
	items, mappings, mailLogin, pass, err := h.listMailboxesWithSpecialRoles(r)
	if err != nil {
		if isSessionMailAuthError(err) {
			h.writeMailAuthError(w, r, err)
			return
		}
		util.WriteError(w, 500, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	actualName := target
	found := false
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Name), target) {
			actualName = strings.TrimSpace(item.Name)
			found = true
			break
		}
	}
	created := false
	if !found {
		if !req.CreateIfMissing {
			util.WriteError(w, 404, "mailbox_not_found", "mailbox does not exist", middleware.RequestID(r.Context()))
			return
		}
		if err := h.svc.Mail().CreateMailbox(r.Context(), mailLogin, pass, target); err != nil {
			util.WriteError(w, 502, "imap_error", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		created = true
	}
	if err := h.svc.Store().UpsertSpecialMailboxMapping(r.Context(), u.ID, mailLogin, role, actualName); err != nil {
		util.WriteError(w, 500, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.invalidateMailCaches(mailLogin)
	mappings[role] = actualName
	items = applySpecialMailboxRoles(items, mappings)
	responseItems := specialMailboxMappingsForResponse(mappings)
	sort.Slice(responseItems, func(i, j int) bool {
		return responseItems[i].Role < responseItems[j].Role
	})
	util.WriteJSON(w, 200, map[string]any{
		"status":       "ok",
		"role":         role,
		"mailbox_name": actualName,
		"created":      created,
		"items":        responseItems,
		"mailboxes":    items,
	})
}
