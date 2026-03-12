package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"despatch/internal/middleware"
	"despatch/internal/models"
	"despatch/internal/store"
	"despatch/internal/util"
)

type contactImportSummary struct {
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Skipped  int      `json:"skipped"`
	Warnings []string `json:"warnings,omitempty"`
}

func dedupeCaseFoldStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeContactEmailsPayload(items []models.ContactEmail) ([]models.ContactEmail, error) {
	if len(items) == 0 {
		return nil, errors.New("at least one email is required")
	}
	out := make([]models.ContactEmail, 0, len(items))
	seen := map[string]struct{}{}
	hasPrimary := false
	for _, item := range items {
		email, err := normalizeRequiredMailboxAddress(item.Email, "email")
		if err != nil {
			return nil, err
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		item.Email = email
		item.Label = strings.TrimSpace(item.Label)
		if item.IsPrimary && !hasPrimary {
			hasPrimary = true
		} else {
			item.IsPrimary = false
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil, errors.New("at least one email is required")
	}
	if !hasPrimary {
		out[0].IsPrimary = true
	}
	return out, nil
}

func primaryContactEmail(items []models.ContactEmail) string {
	for _, item := range items {
		if item.IsPrimary && strings.TrimSpace(item.Email) != "" {
			return strings.TrimSpace(item.Email)
		}
	}
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0].Email)
}

func (h *Handlers) validateContactHints(ctx context.Context, u models.User, preferredAccountID, preferredSenderID string) error {
	if preferredAccountID = strings.TrimSpace(preferredAccountID); preferredAccountID != "" {
		if _, err := h.svc.Store().GetMailAccountByID(ctx, u.ID, preferredAccountID); err != nil {
			return errors.New("preferred_account_id is invalid")
		}
	}
	if preferredSenderID = strings.TrimSpace(preferredSenderID); preferredSenderID != "" {
		if _, err := h.svc.GetSenderProfileByID(ctx, u, preferredSenderID); err != nil {
			return errors.New("preferred_sender_id is invalid")
		}
	}
	return nil
}

func (h *Handlers) normalizeContactWriteInput(ctx context.Context, u models.User, in models.Contact) (models.Contact, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Notes = strings.TrimSpace(in.Notes)
	in.Nicknames = dedupeCaseFoldStrings(in.Nicknames)
	in.GroupIDs = dedupeCaseFoldStrings(in.GroupIDs)
	in.PreferredAccountID = strings.TrimSpace(in.PreferredAccountID)
	in.PreferredSenderID = strings.TrimSpace(in.PreferredSenderID)
	var err error
	in.Emails, err = normalizeContactEmailsPayload(in.Emails)
	if err != nil {
		return models.Contact{}, err
	}
	if in.Name == "" {
		in.Name = primaryContactEmail(in.Emails)
	}
	if err := h.validateContactHints(ctx, u, in.PreferredAccountID, in.PreferredSenderID); err != nil {
		return models.Contact{}, err
	}
	return in, nil
}

func (h *Handlers) normalizeContactGroupWriteInput(in models.ContactGroup) (models.ContactGroup, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.MemberContactIDs = dedupeCaseFoldStrings(in.MemberContactIDs)
	if in.Name == "" {
		return models.ContactGroup{}, errors.New("name is required")
	}
	return in, nil
}

func (h *Handlers) V2ListContacts(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListContacts(r.Context(), u.ID, strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		util.WriteError(w, 500, "contacts_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2GetContact(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	item, err := h.svc.Store().GetContactByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "contact_not_found", "contact not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "contact_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, item)
}

func (h *Handlers) V2CreateContact(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.Contact
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.UserID = u.ID
	normalized, err := h.normalizeContactWriteInput(r.Context(), u, req)
	if err != nil {
		util.WriteError(w, 400, "contact_create_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out, err := h.svc.Store().CreateContact(r.Context(), normalized)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			util.WriteError(w, 400, "contact_create_failed", "group_ids contain unknown contacts group", middleware.RequestID(r.Context()))
		case errors.Is(err, store.ErrConflict):
			util.WriteError(w, 409, "contact_conflict", "contact already exists", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "contact_create_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2UpdateContact(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if _, err := h.svc.Store().GetContactByID(r.Context(), u.ID, chi.URLParam(r, "id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "contact_not_found", "contact not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "contact_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req models.Contact
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = chi.URLParam(r, "id")
	req.UserID = u.ID
	normalized, err := h.normalizeContactWriteInput(r.Context(), u, req)
	if err != nil {
		util.WriteError(w, 400, "contact_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out, err := h.svc.Store().UpdateContact(r.Context(), normalized)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			util.WriteError(w, 400, "contact_update_failed", "group_ids contain unknown contacts group", middleware.RequestID(r.Context()))
		case errors.Is(err, store.ErrConflict):
			util.WriteError(w, 409, "contact_conflict", "contact already exists", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "contact_update_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2DeleteContact(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.Store().DeleteContact(r.Context(), u.ID, chi.URLParam(r, "id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "contact_not_found", "contact not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "contact_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func (h *Handlers) V2ListContactGroups(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListContactGroups(r.Context(), u.ID, strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		util.WriteError(w, 500, "contact_groups_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2GetContactGroup(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	item, err := h.svc.Store().GetContactGroupByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "contact_group_not_found", "contact group not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "contact_group_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, item)
}

func (h *Handlers) V2CreateContactGroup(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.ContactGroup
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.UserID = u.ID
	normalized, err := h.normalizeContactGroupWriteInput(req)
	if err != nil {
		util.WriteError(w, 400, "contact_group_create_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out, err := h.svc.Store().CreateContactGroup(r.Context(), normalized)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			util.WriteError(w, 400, "contact_group_create_failed", "member_contact_ids contain unknown contacts", middleware.RequestID(r.Context()))
		case errors.Is(err, store.ErrConflict):
			util.WriteError(w, 409, "contact_group_conflict", "group already exists", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "contact_group_create_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2UpdateContactGroup(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if _, err := h.svc.Store().GetContactGroupByID(r.Context(), u.ID, chi.URLParam(r, "id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "contact_group_not_found", "contact group not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "contact_group_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req models.ContactGroup
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.ID = chi.URLParam(r, "id")
	req.UserID = u.ID
	normalized, err := h.normalizeContactGroupWriteInput(req)
	if err != nil {
		util.WriteError(w, 400, "contact_group_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out, err := h.svc.Store().UpdateContactGroup(r.Context(), normalized)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			util.WriteError(w, 400, "contact_group_update_failed", "member_contact_ids contain unknown contacts", middleware.RequestID(r.Context()))
		case errors.Is(err, store.ErrConflict):
			util.WriteError(w, 409, "contact_group_conflict", "group already exists", middleware.RequestID(r.Context()))
		default:
			util.WriteError(w, 500, "contact_group_update_failed", err.Error(), middleware.RequestID(r.Context()))
		}
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2DeleteContactGroup(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.Store().DeleteContactGroup(r.Context(), u.ID, chi.URLParam(r, "id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "contact_group_not_found", "contact group not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "contact_group_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "deleted"})
}

func inferContactsFormat(raw, filename string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "csv", "vcf":
		return value
	}
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(filename)))
	if ext == ".csv" {
		return "csv"
	}
	if ext == ".vcf" {
		return "vcf"
	}
	return ""
}

func mergeImportedContact(existing models.Contact, imported models.Contact) models.Contact {
	if strings.TrimSpace(existing.Name) == "" && strings.TrimSpace(imported.Name) != "" {
		existing.Name = imported.Name
	}
	if strings.TrimSpace(existing.Notes) == "" && strings.TrimSpace(imported.Notes) != "" {
		existing.Notes = imported.Notes
	}
	if strings.TrimSpace(existing.PreferredAccountID) == "" && strings.TrimSpace(imported.PreferredAccountID) != "" {
		existing.PreferredAccountID = imported.PreferredAccountID
	}
	if strings.TrimSpace(existing.PreferredSenderID) == "" && strings.TrimSpace(imported.PreferredSenderID) != "" {
		existing.PreferredSenderID = imported.PreferredSenderID
	}
	existing.Nicknames = dedupeCaseFoldStrings(append(existing.Nicknames, imported.Nicknames...))
	existing.GroupIDs = dedupeCaseFoldStrings(append(existing.GroupIDs, imported.GroupIDs...))
	existing.Emails = append(existing.Emails, imported.Emails...)
	return existing
}

func indexContactsByEmail(items []models.Contact) map[string][]string {
	out := map[string][]string{}
	for _, item := range items {
		for _, email := range item.Emails {
			key := strings.ToLower(strings.TrimSpace(email.Email))
			if key == "" {
				continue
			}
			out[key] = append(out[key], item.ID)
		}
	}
	return out
}

func replaceIndexedContact(items []models.Contact, next models.Contact) []models.Contact {
	for i := range items {
		if items[i].ID == next.ID {
			items[i] = next
			return items
		}
	}
	return append(items, next)
}

func resolveImportedGroupIDs(ctx context.Context, st *store.Store, userID string, groupsByName map[string]models.ContactGroup, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	groupIDs := make([]string, 0, len(names))
	for _, name := range dedupeCaseFoldStrings(names) {
		key := strings.ToLower(strings.TrimSpace(name))
		group, ok := groupsByName[key]
		if !ok {
			created, err := st.CreateContactGroup(ctx, models.ContactGroup{
				ID:          uuid.NewString(),
				UserID:      userID,
				Name:        name,
				Description: "",
			})
			if err != nil {
				return nil, err
			}
			group = created
			groupsByName[key] = group
		}
		groupIDs = append(groupIDs, group.ID)
	}
	return groupIDs, nil
}

func (h *Handlers) normalizeImportedContactRecord(ctx context.Context, u models.User, item importedContact, warningPrefix string) (models.Contact, []string, error) {
	warnings := make([]string, 0, 2)
	emails, err := normalizeContactEmailsPayload(item.Emails)
	if err != nil {
		return models.Contact{}, warnings, err
	}
	contact := models.Contact{
		Name:               strings.TrimSpace(item.Name),
		Nicknames:          dedupeCaseFoldStrings(item.Nicknames),
		Emails:             emails,
		Notes:              strings.TrimSpace(item.Notes),
		PreferredAccountID: strings.TrimSpace(item.PreferredAccountID),
		PreferredSenderID:  strings.TrimSpace(item.PreferredSenderID),
	}
	if contact.Name == "" {
		contact.Name = primaryContactEmail(contact.Emails)
	}
	if err := h.validateContactHints(ctx, u, contact.PreferredAccountID, contact.PreferredSenderID); err != nil {
		if contact.PreferredAccountID != "" {
			warnings = append(warnings, warningPrefix+": invalid preferred account ignored")
			contact.PreferredAccountID = ""
		}
		if contact.PreferredSenderID != "" {
			warnings = append(warnings, warningPrefix+": invalid preferred sender ignored")
			contact.PreferredSenderID = ""
		}
	}
	return contact, warnings, nil
}

func (h *Handlers) V2ImportContacts(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		util.WriteError(w, 400, "contacts_import_failed", "invalid multipart upload", middleware.RequestID(r.Context()))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		util.WriteError(w, 400, "contacts_import_failed", "file is required", middleware.RequestID(r.Context()))
		return
	}
	defer file.Close()
	format := inferContactsFormat(r.URL.Query().Get("format"), header.Filename)
	if format == "" {
		util.WriteError(w, 400, "contacts_import_failed", "format must be csv or vcf", middleware.RequestID(r.Context()))
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		util.WriteError(w, 400, "contacts_import_failed", "cannot read uploaded file", middleware.RequestID(r.Context()))
		return
	}
	var records []importedContact
	switch format {
	case "csv":
		records, err = parseContactsCSV(data)
	case "vcf":
		records, err = parseContactsVCF(data)
	default:
		err = errors.New("unsupported format")
	}
	if err != nil {
		util.WriteError(w, 400, "contacts_import_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}

	contacts, err := h.svc.Store().ListContacts(r.Context(), u.ID, "")
	if err != nil {
		util.WriteError(w, 500, "contacts_import_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	groups, err := h.svc.Store().ListContactGroups(r.Context(), u.ID, "")
	if err != nil {
		util.WriteError(w, 500, "contacts_import_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	groupsByName := map[string]models.ContactGroup{}
	for _, group := range groups {
		groupsByName[strings.ToLower(strings.TrimSpace(group.Name))] = group
	}

	summary := contactImportSummary{Warnings: []string{}}
	emailIndex := indexContactsByEmail(contacts)
	for idx, record := range records {
		prefix := "record"
		if format == "csv" {
			prefix = "csv record"
		}
		label := prefix + " " + strconv.Itoa(idx+1)
		normalized, warnings, err := h.normalizeImportedContactRecord(r.Context(), u, record, label)
		if err != nil {
			summary.Skipped++
			summary.Warnings = append(summary.Warnings, label+": "+err.Error())
			continue
		}
		groupIDs, err := resolveImportedGroupIDs(r.Context(), h.svc.Store(), u.ID, groupsByName, record.Groups)
		if err != nil {
			util.WriteError(w, 500, "contacts_import_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		normalized.GroupIDs = groupIDs
		summary.Warnings = append(summary.Warnings, warnings...)

		matchIDs := map[string]struct{}{}
		for _, email := range normalized.Emails {
			for _, contactID := range emailIndex[strings.ToLower(strings.TrimSpace(email.Email))] {
				matchIDs[contactID] = struct{}{}
			}
		}
		switch len(matchIDs) {
		case 0:
			normalized.ID = uuid.NewString()
			normalized.UserID = u.ID
			created, err := h.svc.Store().CreateContact(r.Context(), normalized)
			if err != nil {
				util.WriteError(w, 500, "contacts_import_failed", err.Error(), middleware.RequestID(r.Context()))
				return
			}
			contacts = append(contacts, created)
			emailIndex = indexContactsByEmail(contacts)
			summary.Created++
		case 1:
			var existing models.Contact
			for contactID := range matchIDs {
				for _, item := range contacts {
					if item.ID == contactID {
						existing = item
						break
					}
				}
			}
			merged := mergeImportedContact(existing, normalized)
			merged.UserID = u.ID
			updated, err := h.svc.Store().UpdateContact(r.Context(), merged)
			if err != nil {
				util.WriteError(w, 500, "contacts_import_failed", err.Error(), middleware.RequestID(r.Context()))
				return
			}
			contacts = replaceIndexedContact(contacts, updated)
			emailIndex = indexContactsByEmail(contacts)
			summary.Updated++
		default:
			summary.Skipped++
			summary.Warnings = append(summary.Warnings, label+": skipped because emails match multiple existing contacts")
		}
	}
	util.WriteJSON(w, 200, summary)
}

func (h *Handlers) V2ExportContacts(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	format := inferContactsFormat(r.URL.Query().Get("format"), "")
	if format == "" {
		util.WriteError(w, 400, "contacts_export_failed", "format must be csv or vcf", middleware.RequestID(r.Context()))
		return
	}
	contacts, err := h.svc.Store().ListContacts(r.Context(), u.ID, "")
	if err != nil {
		util.WriteError(w, 500, "contacts_export_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	groups, err := h.svc.Store().ListContactGroups(r.Context(), u.ID, "")
	if err != nil {
		util.WriteError(w, 500, "contacts_export_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	sort.Slice(contacts, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(contacts[i].Name))
		right := strings.ToLower(strings.TrimSpace(contacts[j].Name))
		if left == right {
			return contacts[i].ID < contacts[j].ID
		}
		return left < right
	})
	groupsByID := map[string]models.ContactGroup{}
	for _, group := range groups {
		groupsByID[group.ID] = group
	}
	var data []byte
	switch format {
	case "csv":
		data, err = exportContactsCSV(contacts, groupsByID)
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="despatch-contacts.csv"`)
	case "vcf":
		data, err = exportContactsVCF(contacts, groupsByID)
		w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="despatch-contacts.vcf"`)
	}
	if err != nil {
		util.WriteError(w, 500, "contacts_export_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
