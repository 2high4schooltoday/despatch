package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	netmail "net/mail"
	"sort"
	"strconv"
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

type outboundCampaignWriteRequest struct {
	Name               string          `json:"name"`
	Status             string          `json:"status"`
	GoalKind           string          `json:"goal_kind"`
	PlaybookKey        string          `json:"playbook_key"`
	CampaignMode       string          `json:"campaign_mode"`
	AudienceSourceKind string          `json:"audience_source_kind"`
	AudienceSourceRef  string          `json:"audience_source_ref"`
	SenderPolicyKind   string          `json:"sender_policy_kind"`
	SenderPolicyRef    json.RawMessage `json:"sender_policy_ref"`
	ReplyPolicy        json.RawMessage `json:"reply_policy"`
	SuppressionPolicy  json.RawMessage `json:"suppression_policy"`
	SchedulePolicy     json.RawMessage `json:"schedule_policy"`
	CompliancePolicy   json.RawMessage `json:"compliance_policy"`
	GovernancePolicy   json.RawMessage `json:"governance_policy"`
}

type outboundStepWriteRequest struct {
	Position            int             `json:"position"`
	Kind                string          `json:"kind"`
	ThreadMode          string          `json:"thread_mode"`
	SubjectTemplate     string          `json:"subject_template"`
	BodyTemplate        string          `json:"body_template"`
	WaitIntervalMinutes int             `json:"wait_interval_minutes"`
	SendWindow          json.RawMessage `json:"send_window"`
	TaskPolicy          json.RawMessage `json:"task_policy"`
	BranchPolicy        json.RawMessage `json:"branch_policy"`
	StopIfReplied       *bool           `json:"stop_if_replied"`
	StopIfClicked       *bool           `json:"stop_if_clicked"`
	StopIfBooked        *bool           `json:"stop_if_booked"`
	StopIfUnsubscribed  *bool           `json:"stop_if_unsubscribed"`
}

type outboundAudienceEntry struct {
	ContactID string `json:"contact_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email"`
}

type outboundAudienceRequest struct {
	AudienceSourceKind string                  `json:"audience_source_kind"`
	AudienceSourceRef  string                  `json:"audience_source_ref"`
	ManualRecipients   []string                `json:"manual_recipients"`
	ManualEntries      []outboundAudienceEntry `json:"manual_entries"`
	CSVText            string                  `json:"csv_text"`
}

type outboundReplyOpsActionRequest struct {
	Outcome    string  `json:"outcome,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Action     string  `json:"action,omitempty"`
	ScopeValue string  `json:"scope_value,omitempty"`
	Until      string  `json:"until,omitempty"`
}

func compactJSONOrString(raw json.RawMessage, defaultValue string) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return defaultValue, nil
	}
	if strings.HasPrefix(trimmed, "\"") {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return strings.TrimSpace(value), nil
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(parsed)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func normalizeOutboundStepRequest(req outboundStepWriteRequest) models.OutboundCampaignStep {
	return models.OutboundCampaignStep{
		Position:            req.Position,
		Kind:                req.Kind,
		ThreadMode:          req.ThreadMode,
		SubjectTemplate:     strings.TrimSpace(req.SubjectTemplate),
		BodyTemplate:        strings.TrimSpace(req.BodyTemplate),
		WaitIntervalMinutes: req.WaitIntervalMinutes,
		StopIfReplied:       req.StopIfReplied == nil || *req.StopIfReplied,
		StopIfClicked:       req.StopIfClicked != nil && *req.StopIfClicked,
		StopIfBooked:        req.StopIfBooked != nil && *req.StopIfBooked,
		StopIfUnsubscribed:  req.StopIfUnsubscribed == nil || *req.StopIfUnsubscribed,
	}
}

func parseNullableRFC3339(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid RFC3339 timestamp")
	}
	return parsed.UTC(), nil
}

func (h *Handlers) outboundCampaignFromRequest(req outboundCampaignWriteRequest) (models.OutboundCampaign, error) {
	senderPolicyRef, err := compactJSONOrString(req.SenderPolicyRef, "")
	if err != nil {
		return models.OutboundCampaign{}, fmt.Errorf("invalid sender_policy_ref")
	}
	replyPolicy, err := compactJSONOrString(req.ReplyPolicy, "{}")
	if err != nil {
		return models.OutboundCampaign{}, fmt.Errorf("invalid reply_policy")
	}
	suppressionPolicy, err := compactJSONOrString(req.SuppressionPolicy, "{}")
	if err != nil {
		return models.OutboundCampaign{}, fmt.Errorf("invalid suppression_policy")
	}
	schedulePolicy, err := compactJSONOrString(req.SchedulePolicy, "{}")
	if err != nil {
		return models.OutboundCampaign{}, fmt.Errorf("invalid schedule_policy")
	}
	compliancePolicy, err := compactJSONOrString(req.CompliancePolicy, "{}")
	if err != nil {
		return models.OutboundCampaign{}, fmt.Errorf("invalid compliance_policy")
	}
	governancePolicy, err := compactJSONOrString(req.GovernancePolicy, "{}")
	if err != nil {
		return models.OutboundCampaign{}, fmt.Errorf("invalid governance_policy")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return models.OutboundCampaign{}, fmt.Errorf("name is required")
	}
	return models.OutboundCampaign{
		Name:                  name,
		Status:                req.Status,
		GoalKind:              req.GoalKind,
		PlaybookKey:           strings.TrimSpace(req.PlaybookKey),
		CampaignMode:          req.CampaignMode,
		AudienceSourceKind:    req.AudienceSourceKind,
		AudienceSourceRef:     strings.TrimSpace(req.AudienceSourceRef),
		SenderPolicyKind:      req.SenderPolicyKind,
		SenderPolicyRef:       senderPolicyRef,
		ReplyPolicyJSON:       replyPolicy,
		SuppressionPolicyJSON: suppressionPolicy,
		SchedulePolicyJSON:    schedulePolicy,
		CompliancePolicyJSON:  compliancePolicy,
		GovernancePolicyJSON:  governancePolicy,
	}, nil
}

func splitAudienceRecipientValues(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		for _, part := range strings.FieldsFunc(item, func(r rune) bool {
			return r == ',' || r == '\n' || r == ';'
		}) {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

func contactEmailIndex(items []models.Contact) map[string]models.Contact {
	out := make(map[string]models.Contact, len(items))
	for _, item := range items {
		for _, email := range item.Emails {
			key := strings.ToLower(strings.TrimSpace(email.Email))
			if key == "" {
				continue
			}
			if _, ok := out[key]; !ok {
				out[key] = item
			}
		}
	}
	return out
}

func dedupeAudienceMembers(items []models.OutboundAudienceMember) []models.OutboundAudienceMember {
	out := make([]models.OutboundAudienceMember, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item.RecipientEmail))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ContactName != out[j].ContactName {
			return strings.ToLower(out[i].ContactName) < strings.ToLower(out[j].ContactName)
		}
		return out[i].RecipientEmail < out[j].RecipientEmail
	})
	return out
}

func parseAddressListEmails(raw string) []string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if list, err := netmail.ParseAddressList(value); err == nil && len(list) > 0 {
		out := make([]string, 0, len(list))
		for _, item := range list {
			if item == nil {
				continue
			}
			out = append(out, strings.ToLower(strings.TrimSpace(item.Address)))
		}
		return out
	}
	if item, err := netmail.ParseAddress(value); err == nil && item != nil {
		return []string{strings.ToLower(strings.TrimSpace(item.Address))}
	}
	return nil
}

func (h *Handlers) outboundSavedSearchCandidates(ctx context.Context, u models.User, savedSearchID string) ([]models.OutboundAudienceMember, error) {
	searches, err := h.svc.Store().ListSavedSearches(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	var record models.SavedSearch
	for _, item := range searches {
		if item.ID == strings.TrimSpace(savedSearchID) {
			record = item
			break
		}
	}
	if strings.TrimSpace(record.ID) == "" {
		return nil, store.ErrNotFound
	}
	var parsed struct {
		AccountScope string `json:"account_scope"`
		ViewKind     string `json:"view_kind"`
		Mailbox      string `json:"mailbox"`
		SmartView    string `json:"smart_view"`
		Query        string `json:"query"`
		Filters      struct {
			Query          string   `json:"query"`
			From           string   `json:"from"`
			To             string   `json:"to"`
			Subject        string   `json:"subject"`
			DateFrom       string   `json:"date_from"`
			DateTo         string   `json:"date_to"`
			Unread         bool     `json:"unread"`
			Flagged        bool     `json:"flagged"`
			HasAttachments bool     `json:"has_attachments"`
			Waiting        bool     `json:"waiting"`
			Snoozed        bool     `json:"snoozed"`
			FollowUp       bool     `json:"follow_up"`
			CategoryID     string   `json:"category_id"`
			TagIDs         []string `json:"tag_ids"`
			AccountIDs     []string `json:"account_ids"`
		} `json:"filters"`
	}
	_ = json.Unmarshal([]byte(record.FiltersJSON), &parsed)
	accounts, err := h.svc.Store().ListMailAccounts(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	filteredAccounts, err := resolveIndexedFilteredAccounts(accounts, parsed.Filters.AccountIDs)
	if err != nil {
		return nil, err
	}
	filter := models.IndexedMessageFilter{
		Query:          strings.TrimSpace(firstNonEmpty(parsed.Query, parsed.Filters.Query)),
		From:           strings.TrimSpace(parsed.Filters.From),
		To:             strings.TrimSpace(parsed.Filters.To),
		Subject:        strings.TrimSpace(parsed.Filters.Subject),
		Unread:         parsed.Filters.Unread,
		Flagged:        parsed.Filters.Flagged,
		HasAttachments: parsed.Filters.HasAttachments,
		Waiting:        parsed.Filters.Waiting,
		Snoozed:        parsed.Filters.Snoozed,
		FollowUp:       parsed.Filters.FollowUp,
		CategoryID:     strings.TrimSpace(parsed.Filters.CategoryID),
		TagIDs:         normalizeIndexedFilterTagIDs(parsed.Filters.TagIDs),
		AccountIDs:     normalizeIndexedFilterAccountIDs(parsed.Filters.AccountIDs),
	}
	loc := indexedFilterLocation(ctx, h, u.ID)
	filter.DateFrom, filter.HasDateFrom, _ = parseIndexedCalendarDate(parsed.Filters.DateFrom, loc, false)
	filter.DateTo, filter.HasDateTo, _ = parseIndexedCalendarDate(parsed.Filters.DateTo, loc, true)
	items, _, err := h.queryIndexedMessages(ctx, u, filteredAccounts, strings.TrimSpace(parsed.Mailbox), nil, filter, 1, 500, "desc", true)
	if err != nil {
		return nil, err
	}
	contacts, err := h.svc.Store().ListContacts(ctx, u.ID, "")
	if err != nil {
		return nil, err
	}
	byEmail := contactEmailIndex(contacts)
	selfEmails := map[string]struct{}{strings.ToLower(strings.TrimSpace(u.Email)): {}}
	for _, account := range accounts {
		selfEmails[strings.ToLower(strings.TrimSpace(account.Login))] = struct{}{}
	}
	members := make([]models.OutboundAudienceMember, 0, len(items))
	for _, item := range items {
		froms := parseAddressListEmails(item.FromValue)
		tos := append(parseAddressListEmails(item.ToValue), parseAddressListEmails(item.CCValue)...)
		candidates := make([]string, 0, 4)
		if len(froms) > 0 {
			fromSelf := false
			for _, from := range froms {
				if _, ok := selfEmails[from]; ok {
					fromSelf = true
					break
				}
			}
			if fromSelf {
				for _, to := range tos {
					if _, ok := selfEmails[to]; !ok {
						candidates = append(candidates, to)
					}
				}
			} else {
				candidates = append(candidates, froms...)
			}
		}
		for _, emailValue := range candidates {
			if _, ok := selfEmails[emailValue]; ok {
				continue
			}
			member := models.OutboundAudienceMember{
				RecipientEmail:    emailValue,
				RecipientDomain:   strings.TrimSpace(strings.TrimPrefix(emailValue[strings.LastIndex(emailValue, "@"):], "@")),
				SeedAccountID:     strings.TrimSpace(item.AccountID),
				SeedThreadID:      strings.TrimSpace(item.ThreadID),
				SeedMessageID:     mail.NormalizeMessageIDHeader(item.MessageIDHeader),
				SeedThreadSubject: strings.TrimSpace(item.Subject),
				SeedMailbox:       strings.TrimSpace(item.Mailbox),
				ExistingThread:    strings.TrimSpace(item.ThreadID) != "" || strings.TrimSpace(item.MessageIDHeader) != "",
			}
			if contact, ok := byEmail[emailValue]; ok {
				member.ContactID = contact.ID
				member.ContactName = contact.Name
				member.PreferredAccountID = contact.PreferredAccountID
				member.PreferredSenderID = contact.PreferredSenderID
			}
			members = append(members, member)
		}
	}
	return dedupeAudienceMembers(members), nil
}

func (h *Handlers) resolveOutboundAudienceMembers(ctx context.Context, u models.User, req outboundAudienceRequest) ([]models.OutboundAudienceMember, error) {
	contacts, err := h.svc.Store().ListContacts(ctx, u.ID, "")
	if err != nil {
		return nil, err
	}
	byEmail := contactEmailIndex(contacts)
	switch strings.ToLower(strings.TrimSpace(req.AudienceSourceKind)) {
	case "contact_group":
		group, err := h.svc.Store().GetContactGroupByID(ctx, u.ID, req.AudienceSourceRef)
		if err != nil {
			return nil, err
		}
		members := make([]models.OutboundAudienceMember, 0, len(group.MemberContactIDs))
		for _, contactID := range group.MemberContactIDs {
			contact, err := h.svc.Store().GetContactByID(ctx, u.ID, contactID)
			if err != nil {
				continue
			}
			for _, emailValue := range contact.Emails {
				email := strings.ToLower(strings.TrimSpace(emailValue.Email))
				if email == "" {
					continue
				}
				members = append(members, models.OutboundAudienceMember{
					ContactID:          contact.ID,
					ContactName:        strings.TrimSpace(contact.Name),
					RecipientEmail:     email,
					RecipientDomain:    strings.TrimSpace(strings.TrimPrefix(email[strings.LastIndex(email, "@"):], "@")),
					PreferredAccountID: contact.PreferredAccountID,
					PreferredSenderID:  contact.PreferredSenderID,
				})
			}
		}
		return dedupeAudienceMembers(members), nil
	case "saved_search":
		return h.outboundSavedSearchCandidates(ctx, u, req.AudienceSourceRef)
	case "csv_import":
		reader := csv.NewReader(strings.NewReader(req.CSVText))
		reader.FieldsPerRecord = -1
		rows, err := reader.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("invalid CSV input")
		}
		members := make([]models.OutboundAudienceMember, 0, len(rows))
		start := 0
		headerEmailCol := -1
		headerNameCol := -1
		headerContactCol := -1
		if len(rows) > 0 {
			for idx, cell := range rows[0] {
				key := strings.ToLower(strings.TrimSpace(cell))
				switch key {
				case "email", "recipient_email":
					headerEmailCol = idx
				case "name", "full_name":
					headerNameCol = idx
				case "contact_id":
					headerContactCol = idx
				}
			}
			if headerEmailCol >= 0 {
				start = 1
			}
		}
		for _, row := range rows[start:] {
			if len(row) == 0 {
				continue
			}
			emailCell := ""
			nameCell := ""
			contactID := ""
			if headerEmailCol >= 0 {
				if headerEmailCol < len(row) {
					emailCell = row[headerEmailCol]
				}
				if headerNameCol >= 0 && headerNameCol < len(row) {
					nameCell = row[headerNameCol]
				}
				if headerContactCol >= 0 && headerContactCol < len(row) {
					contactID = row[headerContactCol]
				}
			} else {
				emailCell = row[0]
				if len(row) > 1 {
					nameCell = row[1]
				}
			}
			email, err := normalizeRequiredMailboxAddress(emailCell, "email")
			if err != nil {
				continue
			}
			member := models.OutboundAudienceMember{
				ContactID:       strings.TrimSpace(contactID),
				ContactName:     strings.TrimSpace(nameCell),
				RecipientEmail:  email,
				RecipientDomain: strings.TrimSpace(strings.TrimPrefix(email[strings.LastIndex(email, "@"):], "@")),
			}
			if contact, ok := byEmail[email]; ok {
				member.ContactID = contact.ID
				member.ContactName = firstNonEmpty(member.ContactName, contact.Name)
				member.PreferredAccountID = contact.PreferredAccountID
				member.PreferredSenderID = contact.PreferredSenderID
			}
			members = append(members, member)
		}
		return dedupeAudienceMembers(members), nil
	default:
		members := make([]models.OutboundAudienceMember, 0, len(req.ManualRecipients)+len(req.ManualEntries))
		for _, value := range splitAudienceRecipientValues(req.ManualRecipients) {
			email, err := normalizeRequiredMailboxAddress(value, "email")
			if err != nil {
				continue
			}
			member := models.OutboundAudienceMember{
				RecipientEmail:  email,
				RecipientDomain: strings.TrimSpace(strings.TrimPrefix(email[strings.LastIndex(email, "@"):], "@")),
			}
			if contact, ok := byEmail[email]; ok {
				member.ContactID = contact.ID
				member.ContactName = contact.Name
				member.PreferredAccountID = contact.PreferredAccountID
				member.PreferredSenderID = contact.PreferredSenderID
			}
			members = append(members, member)
		}
		for _, entry := range req.ManualEntries {
			email, err := normalizeRequiredMailboxAddress(entry.Email, "email")
			if err != nil {
				continue
			}
			member := models.OutboundAudienceMember{
				ContactID:       strings.TrimSpace(entry.ContactID),
				ContactName:     strings.TrimSpace(entry.Name),
				RecipientEmail:  email,
				RecipientDomain: strings.TrimSpace(strings.TrimPrefix(email[strings.LastIndex(email, "@"):], "@")),
			}
			if contact, ok := byEmail[email]; ok {
				member.ContactID = firstNonEmpty(member.ContactID, contact.ID)
				member.ContactName = firstNonEmpty(member.ContactName, contact.Name)
				member.PreferredAccountID = contact.PreferredAccountID
				member.PreferredSenderID = contact.PreferredSenderID
			}
			members = append(members, member)
		}
		return dedupeAudienceMembers(members), nil
	}
}

func (h *Handlers) enrichOutboundAudience(ctx context.Context, u models.User, campaignID string, members []models.OutboundAudienceMember) ([]models.OutboundAudienceMember, error) {
	out := make([]models.OutboundAudienceMember, 0, len(members))
	now := time.Now().UTC()
	for _, member := range members {
		if member.RecipientEmail == "" {
			continue
		}
		if existing, err := h.svc.Store().GetOutboundEnrollmentByCampaignEmail(ctx, campaignID, member.RecipientEmail); err == nil {
			member.ExistingEnrollmentID = existing.ID
		}
		if suppression, err := h.svc.Store().GetActiveOutboundSuppression(ctx, u.ID, member.RecipientEmail, member.RecipientDomain, campaignID, now); err == nil {
			member.Suppressed = true
			member.SuppressionReason = firstNonEmpty(suppression.Reason, "suppressed")
		} else if err != nil && err != store.ErrNotFound {
			return nil, err
		}
		if active, err := h.svc.Store().ListActiveOutboundEnrollmentsByRecipient(ctx, u.ID, member.RecipientEmail, campaignID); err == nil && len(active) > 0 {
			member.ActiveElsewhere = true
		}
		out = append(out, member)
	}
	return out, nil
}

func (h *Handlers) V2ListOutboundCampaigns(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListOutboundCampaigns(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "outbound_campaigns_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2ListOutboundPlaybooks(w http.ResponseWriter, r *http.Request) {
	util.WriteJSON(w, 200, map[string]any{"items": h.svc.OutboundPlaybooks()})
}

func (h *Handlers) V2CreateOutboundCampaign(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req outboundCampaignWriteRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	item, err := h.outboundCampaignFromRequest(req)
	if err != nil {
		util.WriteError(w, 400, "outbound_campaign_create_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	item.UserID = u.ID
	out, err := h.svc.Store().CreateOutboundCampaign(r.Context(), item)
	if err != nil {
		util.WriteError(w, 500, "outbound_campaign_create_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2GetOutboundCampaign(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	item, err := h.svc.Store().GetOutboundCampaignByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_campaign_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, item)
}

func (h *Handlers) V2UpdateOutboundCampaign(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req outboundCampaignWriteRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	item, err := h.outboundCampaignFromRequest(req)
	if err != nil {
		util.WriteError(w, 400, "outbound_campaign_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	item.ID = chi.URLParam(r, "id")
	item.UserID = u.ID
	out, err := h.svc.Store().UpdateOutboundCampaign(r.Context(), item)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_campaign_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2LaunchOutboundCampaign(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	issues, err := h.svc.LaunchOutboundCampaign(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_campaign_launch_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	blocking := false
	for _, item := range issues {
		if item.Blocking {
			blocking = true
			break
		}
	}
	status := 200
	if blocking {
		status = 409
	}
	util.WriteJSON(w, status, map[string]any{"issues": issues, "launched": !blocking})
}

func (h *Handlers) V2ApplyOutboundPlaybook(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		PlaybookKey  string `json:"playbook_key"`
		ReplaceSteps *bool  `json:"replace_steps"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	replaceSteps := req.ReplaceSteps != nil && *req.ReplaceSteps
	campaign, steps, err := h.svc.ApplyOutboundPlaybook(r.Context(), u, chi.URLParam(r, "id"), req.PlaybookKey, replaceSteps)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_playbook_apply_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{
		"campaign": campaign,
		"steps":    steps,
	})
}

func (h *Handlers) V2PauseOutboundCampaign(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.Store().SetOutboundCampaignStatus(r.Context(), u.ID, chi.URLParam(r, "id"), "paused", time.Time{}, time.Time{}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_campaign_pause_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "paused"})
}

func (h *Handlers) V2ResumeOutboundCampaign(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.Store().SetOutboundCampaignStatus(r.Context(), u.ID, chi.URLParam(r, "id"), "running", time.Time{}, time.Time{}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_campaign_resume_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "running"})
}

func (h *Handlers) V2ArchiveOutboundCampaign(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.Store().SetOutboundCampaignStatus(r.Context(), u.ID, chi.URLParam(r, "id"), "archived", time.Time{}, time.Now().UTC()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_campaign_archive_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "archived"})
}

func (h *Handlers) V2ListOutboundCampaignSteps(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	campaignID := chi.URLParam(r, "id")
	if _, err := h.svc.Store().GetOutboundCampaignByID(r.Context(), u.ID, campaignID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_steps_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	items, err := h.svc.Store().ListOutboundCampaignSteps(r.Context(), campaignID)
	if err != nil {
		util.WriteError(w, 500, "outbound_steps_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2ListOutboundCampaignEvents(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	campaignID := chi.URLParam(r, "id")
	if _, err := h.svc.Store().GetOutboundCampaignByID(r.Context(), u.ID, campaignID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_events_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 2000 {
			limit = parsed
		}
	}
	items, err := h.svc.Store().ListOutboundCampaignEvents(r.Context(), campaignID, limit)
	if err != nil {
		util.WriteError(w, 500, "outbound_events_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2CreateOutboundCampaignStep(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	campaignID := chi.URLParam(r, "id")
	if _, err := h.svc.Store().GetOutboundCampaignByID(r.Context(), u.ID, campaignID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_step_create_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req outboundStepWriteRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	step := normalizeOutboundStepRequest(req)
	sendWindow, err := compactJSONOrString(req.SendWindow, "{}")
	if err != nil {
		util.WriteError(w, 400, "outbound_step_create_failed", "invalid send_window", middleware.RequestID(r.Context()))
		return
	}
	taskPolicy, err := compactJSONOrString(req.TaskPolicy, "{}")
	if err != nil {
		util.WriteError(w, 400, "outbound_step_create_failed", "invalid task_policy", middleware.RequestID(r.Context()))
		return
	}
	branchPolicy, err := compactJSONOrString(req.BranchPolicy, "{}")
	if err != nil {
		util.WriteError(w, 400, "outbound_step_create_failed", "invalid branch_policy", middleware.RequestID(r.Context()))
		return
	}
	step.CampaignID = campaignID
	step.SendWindowJSON = sendWindow
	step.TaskPolicyJSON = taskPolicy
	step.BranchPolicyJSON = branchPolicy
	out, err := h.svc.Store().CreateOutboundCampaignStep(r.Context(), step)
	if err != nil {
		util.WriteError(w, 500, "outbound_step_create_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2UpdateOutboundCampaignStep(w http.ResponseWriter, r *http.Request) {
	var req outboundStepWriteRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	step := normalizeOutboundStepRequest(req)
	sendWindow, err := compactJSONOrString(req.SendWindow, "{}")
	if err != nil {
		util.WriteError(w, 400, "outbound_step_update_failed", "invalid send_window", middleware.RequestID(r.Context()))
		return
	}
	taskPolicy, err := compactJSONOrString(req.TaskPolicy, "{}")
	if err != nil {
		util.WriteError(w, 400, "outbound_step_update_failed", "invalid task_policy", middleware.RequestID(r.Context()))
		return
	}
	branchPolicy, err := compactJSONOrString(req.BranchPolicy, "{}")
	if err != nil {
		util.WriteError(w, 400, "outbound_step_update_failed", "invalid branch_policy", middleware.RequestID(r.Context()))
		return
	}
	step.ID = chi.URLParam(r, "step_id")
	step.SendWindowJSON = sendWindow
	step.TaskPolicyJSON = taskPolicy
	step.BranchPolicyJSON = branchPolicy
	out, err := h.svc.Store().UpdateOutboundCampaignStep(r.Context(), step)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_step_not_found", "step not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_step_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2DeleteOutboundCampaignStep(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Store().DeleteOutboundCampaignStep(r.Context(), chi.URLParam(r, "step_id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_step_not_found", "step not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_step_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"deleted": true})
}

func (h *Handlers) V2ReorderOutboundCampaignSteps(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StepIDs []string `json:"step_ids"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if err := h.svc.Store().ReorderOutboundCampaignSteps(r.Context(), chi.URLParam(r, "id"), req.StepIDs); err != nil {
		util.WriteError(w, 500, "outbound_step_reorder_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"reordered": true})
}

func (h *Handlers) V2PreviewOutboundAudience(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req outboundAudienceRequest
	if err := decodeJSON(w, r, &req, jsonLimitLarge, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	items, err := h.resolveOutboundAudienceMembers(r.Context(), u, req)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_audience_source_not_found", "audience source not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 400, "outbound_audience_preview_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	items, err = h.enrichOutboundAudience(r.Context(), u, chi.URLParam(r, "id"), items)
	if err != nil {
		util.WriteError(w, 500, "outbound_audience_preview_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2ImportOutboundEnrollments(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	campaignID := chi.URLParam(r, "id")
	campaign, err := h.svc.Store().GetOutboundCampaignByID(r.Context(), u.ID, campaignID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_enrollments_import_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req outboundAudienceRequest
	if err := decodeJSON(w, r, &req, jsonLimitLarge, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	items, err := h.resolveOutboundAudienceMembers(r.Context(), u, req)
	if err != nil {
		util.WriteError(w, 400, "outbound_enrollments_import_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	items, err = h.enrichOutboundAudience(r.Context(), u, campaignID, items)
	if err != nil {
		util.WriteError(w, 500, "outbound_enrollments_import_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	created := 0
	updated := 0
	for _, item := range items {
		status := "pending"
		nextAction := time.Time{}
		if campaign.Status == "running" {
			status = "scheduled"
			nextAction = time.Now().UTC()
		}
		record := models.OutboundEnrollment{
			CampaignID:        campaignID,
			ContactID:         item.ContactID,
			RecipientEmail:    item.RecipientEmail,
			RecipientDomain:   item.RecipientDomain,
			SenderAccountID:   strings.TrimSpace(item.SeedAccountID),
			Status:            status,
			ManualOwnerUserID: u.ID,
			NextActionAt:      nextAction,
		}
		if item.SeedAccountID != "" || item.SeedThreadID != "" || item.SeedMessageID != "" || item.SeedThreadSubject != "" || item.SeedMailbox != "" {
			seedContext, _ := json.Marshal(map[string]any{
				"account_id":     strings.TrimSpace(item.SeedAccountID),
				"thread_id":      strings.TrimSpace(item.SeedThreadID),
				"message_id":     strings.TrimSpace(item.SeedMessageID),
				"thread_subject": strings.TrimSpace(item.SeedThreadSubject),
				"mailbox":        strings.TrimSpace(item.SeedMailbox),
			})
			record.SeedContextJSON = string(seedContext)
		}
		beforeExisting := item.ExistingEnrollmentID != ""
		out, err := h.svc.Store().UpsertOutboundEnrollment(r.Context(), record)
		if err != nil {
			util.WriteError(w, 500, "outbound_enrollments_import_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		if strings.EqualFold(strings.TrimSpace(campaign.CampaignMode), "existing_threads") && (item.SeedAccountID != "" || item.SeedThreadID != "" || item.SeedMessageID != "") {
			binding, _ := h.svc.Store().GetMailThreadBindingByEnrollment(r.Context(), out.ID)
			if strings.TrimSpace(binding.ID) == "" {
				binding = models.MailThreadBinding{
					AccountID:       strings.TrimSpace(item.SeedAccountID),
					BindingType:     "manual",
					CampaignID:      campaignID,
					EnrollmentID:    out.ID,
					OwnerUserID:     u.ID,
					RecipientEmail:  out.RecipientEmail,
					RecipientDomain: out.RecipientDomain,
					ReplyAccountID:  strings.TrimSpace(item.SeedAccountID),
				}
			}
			binding.AccountID = firstNonEmpty(binding.AccountID, strings.TrimSpace(item.SeedAccountID))
			binding.ThreadID = firstNonEmpty(binding.ThreadID, strings.TrimSpace(item.SeedThreadID))
			binding.CampaignID = campaignID
			binding.EnrollmentID = out.ID
			binding.OwnerUserID = u.ID
			binding.RecipientEmail = out.RecipientEmail
			binding.RecipientDomain = out.RecipientDomain
			if strings.TrimSpace(binding.RootOutboundMessageID) == "" {
				binding.RootOutboundMessageID = strings.TrimSpace(item.SeedMessageID)
			}
			if strings.TrimSpace(item.SeedMessageID) != "" {
				binding.LastOutboundMessageID = strings.TrimSpace(item.SeedMessageID)
			}
			binding.ThreadSubject = firstNonEmpty(binding.ThreadSubject, strings.TrimSpace(item.SeedThreadSubject))
			binding.ReplyAccountID = firstNonEmpty(binding.ReplyAccountID, strings.TrimSpace(item.SeedAccountID))
			savedBinding, err := h.svc.Store().UpsertMailThreadBinding(r.Context(), binding)
			if err != nil {
				util.WriteError(w, 500, "outbound_enrollments_import_failed", err.Error(), middleware.RequestID(r.Context()))
				return
			}
			out.ThreadBindingID = savedBinding.ID
			if out.SenderAccountID == "" {
				out.SenderAccountID = strings.TrimSpace(item.SeedAccountID)
			}
			if _, err := h.svc.Store().UpsertOutboundEnrollment(r.Context(), out); err != nil {
				util.WriteError(w, 500, "outbound_enrollments_import_failed", err.Error(), middleware.RequestID(r.Context()))
				return
			}
		}
		payload, _ := json.Marshal(map[string]any{
			"recipient_email": out.RecipientEmail,
			"audience_source": req.AudienceSourceKind,
			"existing_thread": item.ExistingThread,
		})
		_, _ = h.svc.Store().AppendOutboundEvent(r.Context(), models.OutboundEvent{
			CampaignID:       campaignID,
			EnrollmentID:     out.ID,
			EventKind:        "enrolled",
			EventPayloadJSON: string(payload),
			ActorKind:        "user",
			ActorRef:         u.ID,
		})
		if beforeExisting {
			updated++
		} else {
			created++
		}
	}
	util.WriteJSON(w, 200, map[string]any{
		"created": created,
		"updated": updated,
	})
}

func (h *Handlers) V2ListOutboundEnrollments(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListOutboundEnrollmentsByCampaign(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		util.WriteError(w, 500, "outbound_enrollments_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2GetOutboundEnrollment(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	item, err := h.svc.Store().GetOutboundEnrollmentByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_enrollment_not_found", "enrollment not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_enrollment_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, item)
}

func (h *Handlers) V2UpdateOutboundEnrollment(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	current, err := h.svc.Store().GetOutboundEnrollmentByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_enrollment_not_found", "enrollment not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_enrollment_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		Status            *string `json:"status"`
		ManualOwnerUserID *string `json:"manual_owner_user_id"`
		NextActionAt      *string `json:"next_action_at"`
		PauseReason       *string `json:"pause_reason"`
		StopReason        *string `json:"stop_reason"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if req.Status != nil {
		current.Status = strings.TrimSpace(*req.Status)
	}
	if req.ManualOwnerUserID != nil {
		current.ManualOwnerUserID = strings.TrimSpace(*req.ManualOwnerUserID)
	}
	if req.NextActionAt != nil {
		value, err := parseNullableRFC3339(*req.NextActionAt)
		if err != nil {
			util.WriteError(w, 400, "outbound_enrollment_update_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		current.NextActionAt = value
	}
	if req.PauseReason != nil {
		current.PauseReason = strings.TrimSpace(*req.PauseReason)
	}
	if req.StopReason != nil {
		current.StopReason = strings.TrimSpace(*req.StopReason)
	}
	out, err := h.svc.Store().UpsertOutboundEnrollment(r.Context(), current)
	if err != nil {
		util.WriteError(w, 500, "outbound_enrollment_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2PauseOutboundEnrollment(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.ApplyReplyOpsAction(r.Context(), u, chi.URLParam(r, "id"), "pause", "", time.Time{}); err != nil {
		util.WriteError(w, 500, "outbound_enrollment_pause_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "paused"})
}

func (h *Handlers) V2ResumeOutboundEnrollment(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.ApplyReplyOpsAction(r.Context(), u, chi.URLParam(r, "id"), "resume", "", time.Time{}); err != nil {
		util.WriteError(w, 500, "outbound_enrollment_resume_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "scheduled"})
}

func (h *Handlers) V2StopOutboundEnrollment(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.ApplyReplyOpsAction(r.Context(), u, chi.URLParam(r, "id"), "stop", "", time.Time{}); err != nil {
		util.WriteError(w, 500, "outbound_enrollment_stop_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"status": "stopped"})
}

func (h *Handlers) V2AssignOutboundEnrollment(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req struct {
		ManualOwnerUserID string `json:"manual_owner_user_id"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	current, err := h.svc.Store().GetOutboundEnrollmentByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		util.WriteError(w, 500, "outbound_enrollment_assign_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	current.ManualOwnerUserID = firstNonEmpty(req.ManualOwnerUserID, u.ID)
	current.Status = "manual_only"
	out, err := h.svc.Store().UpsertOutboundEnrollment(r.Context(), current)
	if err != nil {
		util.WriteError(w, 500, "outbound_enrollment_assign_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2PreflightOutboundCampaign(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	campaign, err := h.svc.Store().GetOutboundCampaignByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_campaign_not_found", "campaign not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_preflight_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	issues, err := h.svc.PreflightOutboundCampaign(r.Context(), u, campaign)
	if err != nil {
		util.WriteError(w, 500, "outbound_preflight_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"issues": issues})
}

func shouldIncludeReplyOpsEnrollment(item models.OutboundEnrollment) bool {
	if strings.TrimSpace(item.ReplyOutcome) != "" {
		return true
	}
	return strings.TrimSpace(item.LastReplyMessageID) != ""
}

func buildReplyOpsItem(item models.OutboundEnrollment, binding models.MailThreadBinding) models.ReplyOpsItem {
	replyAccountID := strings.TrimSpace(binding.ReplyAccountID)
	if replyAccountID == "" {
		replyAccountID = strings.TrimSpace(binding.CollectorAccountID)
	}
	if replyAccountID == "" {
		replyAccountID = strings.TrimSpace(binding.AccountID)
	}
	replyItem := models.ReplyOpsItem{
		ID:                 item.ID,
		Bucket:             item.LastReplyBucket,
		CampaignID:         item.CampaignID,
		CampaignName:       item.CampaignName,
		EnrollmentID:       item.ID,
		ThreadBindingID:    binding.ID,
		RecipientEmail:     item.RecipientEmail,
		RecipientDomain:    item.RecipientDomain,
		SenderAccountID:    item.SenderAccountID,
		SenderAccountLabel: item.SenderAccountLabel,
		ReplyAccountID:     replyAccountID,
		SenderProfileID:    item.SenderProfileID,
		SenderProfileName:  item.SenderProfileName,
		ReplyOutcome:       item.ReplyOutcome,
		ReplyConfidence:    item.ReplyConfidence,
		MessageID:          item.LastReplyMessageID,
		ThreadID:           binding.ThreadID,
		ThreadSubject:      firstNonEmpty(binding.ThreadSubject, item.ThreadSubject),
		RecommendedAction:  item.Status,
		NeedsHuman:         item.Status == "manual_only" || item.Status == "paused",
		CreatedAt:          item.CreatedAt,
		LastReplyAt:        item.LastReplyAt,
		Status:             item.Status,
	}
	if replyItem.Bucket == "" {
		replyItem.Bucket = "needs_review"
	}
	return replyItem
}

func (h *Handlers) collectReplyOpsItems(ctx context.Context, u models.User) ([]models.ReplyOpsItem, error) {
	campaigns, err := h.svc.Store().ListOutboundCampaigns(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	items := make([]models.ReplyOpsItem, 0, 64)
	for _, campaign := range campaigns {
		enrollments, err := h.svc.Store().ListOutboundEnrollmentsByCampaign(ctx, u.ID, campaign.ID)
		if err != nil {
			return nil, err
		}
		for _, enrollment := range enrollments {
			if !shouldIncludeReplyOpsEnrollment(enrollment) {
				continue
			}
			binding, _ := h.svc.Store().GetMailThreadBindingByEnrollment(ctx, enrollment.ID)
			replyItem := buildReplyOpsItem(enrollment, binding)
			if strings.TrimSpace(binding.LastReplyMessageID) != "" && strings.TrimSpace(replyItem.ReplyAccountID) != "" {
				if msg, err := h.svc.Store().GetIndexedMessageByID(ctx, replyItem.ReplyAccountID, binding.LastReplyMessageID); err == nil {
					replyItem.Preview = firstNonEmpty(strings.TrimSpace(msg.Snippet), strings.TrimSpace(msg.BodyText))
					replyItem.ThreadSubject = firstNonEmpty(replyItem.ThreadSubject, msg.Subject)
				}
			}
			items = append(items, replyItem)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i].LastReplyAt
		if left.IsZero() {
			left = items[i].CreatedAt
		}
		right := items[j].LastReplyAt
		if right.IsZero() {
			right = items[j].CreatedAt
		}
		return left.After(right)
	})
	return items, nil
}

func (h *Handlers) V2ListReplyOpsQueue(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.collectReplyOpsItems(r.Context(), u)
	if err != nil {
		util.WriteError(w, 500, "reply_ops_queue_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2ListReplyOpsBucket(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	bucket := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "bucket")))
	items, err := h.collectReplyOpsItems(r.Context(), u)
	if err != nil {
		util.WriteError(w, 500, "reply_ops_queue_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	filtered := make([]models.ReplyOpsItem, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(item.Bucket, bucket) {
			filtered = append(filtered, item)
		}
	}
	util.WriteJSON(w, 200, map[string]any{"items": filtered})
}

func (h *Handlers) V2GetReplyOpsItem(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	enrollment, err := h.svc.Store().GetOutboundEnrollmentByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "reply_ops_item_not_found", "reply ops item not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "reply_ops_item_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	binding, _ := h.svc.Store().GetMailThreadBindingByEnrollment(r.Context(), enrollment.ID)
	item := buildReplyOpsItem(enrollment, binding)
	if strings.TrimSpace(binding.LastReplyMessageID) != "" && strings.TrimSpace(item.ReplyAccountID) != "" {
		if msg, err := h.svc.Store().GetIndexedMessageByID(r.Context(), item.ReplyAccountID, binding.LastReplyMessageID); err == nil {
			item.Preview = firstNonEmpty(strings.TrimSpace(msg.Snippet), strings.TrimSpace(msg.BodyText))
			item.ThreadSubject = firstNonEmpty(item.ThreadSubject, msg.Subject)
		}
	}
	util.WriteJSON(w, 200, item)
}

func (h *Handlers) V2ClassifyReplyOpsItem(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req outboundReplyOpsActionRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if strings.TrimSpace(req.Outcome) == "" {
		util.WriteError(w, 400, "reply_ops_classify_failed", "outcome is required", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.ApplyManualReplyOutcome(r.Context(), u, chi.URLParam(r, "id"), req.Outcome, req.Confidence); err != nil {
		util.WriteError(w, 500, "reply_ops_classify_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"classified": true})
}

func (h *Handlers) V2TakeoverReplyOpsItem(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.ApplyReplyOpsAction(r.Context(), u, chi.URLParam(r, "id"), "takeover", "", time.Time{}); err != nil {
		util.WriteError(w, 500, "reply_ops_takeover_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"taken_over": true})
}

func (h *Handlers) V2ApplyReplyOpsAction(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req outboundReplyOpsActionRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	until, err := parseNullableRFC3339(req.Until)
	if err != nil {
		util.WriteError(w, 400, "reply_ops_action_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.ApplyReplyOpsAction(r.Context(), u, chi.URLParam(r, "id"), req.Action, req.ScopeValue, until); err != nil {
		util.WriteError(w, 500, "reply_ops_action_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"applied": true})
}

func (h *Handlers) V2ListOutboundRecipients(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListRecipientStates(r.Context(), u.ID, strings.TrimSpace(r.URL.Query().Get("q")))
	if err != nil {
		util.WriteError(w, 500, "outbound_recipients_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2GetOutboundRecipient(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	item, err := h.svc.Store().GetRecipientState(r.Context(), u.ID, chi.URLParam(r, "email"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_recipient_not_found", "recipient not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_recipient_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, item)
}

func (h *Handlers) V2UpdateOutboundRecipient(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	current, err := h.svc.Store().GetRecipientState(r.Context(), u.ID, chi.URLParam(r, "email"))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		util.WriteError(w, 500, "outbound_recipient_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req models.RecipientState
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		current = models.RecipientState{UserID: u.ID, RecipientEmail: chi.URLParam(r, "email")}
	}
	current.UserID = u.ID
	current.RecipientEmail = chi.URLParam(r, "email")
	if strings.TrimSpace(req.Status) != "" {
		current.Status = req.Status
	}
	if strings.TrimSpace(req.Scope) != "" {
		current.Scope = req.Scope
	}
	if strings.TrimSpace(req.SuppressionReason) != "" {
		current.SuppressionReason = req.SuppressionReason
	}
	if strings.TrimSpace(req.Notes) != "" {
		current.Notes = req.Notes
	}
	out, err := h.svc.Store().UpsertRecipientState(r.Context(), current)
	if err != nil {
		util.WriteError(w, 500, "outbound_recipient_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, out)
}

func (h *Handlers) V2ListOutboundSuppressions(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListOutboundSuppressions(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "outbound_suppressions_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"items": items})
}

func (h *Handlers) V2CreateOutboundSuppression(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req models.OutboundSuppression
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	req.UserID = u.ID
	out, err := h.svc.Store().UpsertOutboundSuppression(r.Context(), req)
	if err != nil {
		util.WriteError(w, 500, "outbound_suppression_create_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 201, out)
}

func (h *Handlers) V2DeleteOutboundSuppression(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	if err := h.svc.Store().DeleteOutboundSuppression(r.Context(), u.ID, chi.URLParam(r, "id")); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, 404, "outbound_suppression_not_found", "suppression not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, 500, "outbound_suppression_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, 200, map[string]any{"deleted": true})
}

func (h *Handlers) V2OutboundSenderDiagnostics(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accounts, err := h.svc.Store().ListMailAccounts(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "outbound_sender_diagnostics_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	campaigns, err := h.svc.Store().ListOutboundCampaigns(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "outbound_sender_diagnostics_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	accountByID := make(map[string]models.MailAccount, len(accounts))
	for _, account := range accounts {
		accountByID[account.ID] = account
	}
	funnels, err := h.svc.Store().ListReplyFunnels(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "outbound_sender_diagnostics_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	type funnelTopology struct {
		collectorID    string
		collectorLabel string
		replyTopology  string
	}
	topologies := map[string]funnelTopology{}
	for _, funnel := range funnels {
		collector := accountByID[strings.TrimSpace(funnel.CollectorAccountID)]
		rows, err := h.svc.Store().ListReplyFunnelAccounts(r.Context(), funnel.ID)
		if err != nil {
			util.WriteError(w, 500, "outbound_sender_diagnostics_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		replyTopology := "direct"
		if strings.TrimSpace(funnel.CollectorAccountID) != "" {
			replyTopology = "collector"
			if strings.EqualFold(strings.TrimSpace(funnel.ReplyMode), "smart") {
				replyTopology = "smart"
			}
		}
		for _, row := range rows {
			if !strings.EqualFold(strings.TrimSpace(row.Role), "source") {
				continue
			}
			topologies[row.AccountID] = funnelTopology{
				collectorID:    strings.TrimSpace(funnel.CollectorAccountID),
				collectorLabel: firstNonEmpty(strings.TrimSpace(collector.DisplayName), strings.TrimSpace(collector.Login)),
				replyTopology:  replyTopology,
			}
		}
	}
	type senderStats struct {
		replies  int
		bounces  int
		sends    int
		waiting  int
		bindings int
	}
	stats := map[string]*senderStats{}
	for _, account := range accounts {
		stats[account.ID] = &senderStats{}
	}
	for _, campaign := range campaigns {
		enrollments, err := h.svc.Store().ListOutboundEnrollmentsByCampaign(r.Context(), u.ID, campaign.ID)
		if err != nil {
			util.WriteError(w, 500, "outbound_sender_diagnostics_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		events, err := h.svc.Store().ListOutboundCampaignEvents(r.Context(), campaign.ID, 5000)
		if err != nil {
			util.WriteError(w, 500, "outbound_sender_diagnostics_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		for _, item := range enrollments {
			if stats[item.SenderAccountID] == nil {
				stats[item.SenderAccountID] = &senderStats{}
			}
			if item.Status == "waiting_reply" {
				stats[item.SenderAccountID].waiting++
			}
			if item.ReplyOutcome != "" {
				stats[item.SenderAccountID].replies++
			}
			if item.Status == "bounced" {
				stats[item.SenderAccountID].bounces++
			}
			if strings.TrimSpace(item.ThreadBindingID) != "" {
				stats[item.SenderAccountID].bindings++
			}
		}
		for _, item := range events {
			if item.EventKind != "step_sent" || item.CreatedAt.Before(time.Now().UTC().Add(-24*time.Hour)) {
				continue
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(item.EventPayloadJSON), &payload); err != nil {
				continue
			}
			accountID := strings.TrimSpace(fmt.Sprint(payload["sender_account_id"]))
			if stats[accountID] == nil {
				stats[accountID] = &senderStats{}
			}
			stats[accountID].sends++
		}
	}
	out := make([]models.OutboundSenderDiagnostic, 0, len(accounts))
	for _, account := range accounts {
		stat := stats[account.ID]
		if stat == nil {
			stat = &senderStats{}
		}
		dailyCap, hourlyCap, gapSeconds := service.OutboundProviderPacing(account)
		topology := topologies[account.ID]
		out = append(out, models.OutboundSenderDiagnostic{
			AccountID:             account.ID,
			AccountLabel:          firstNonEmpty(strings.TrimSpace(account.DisplayName), strings.TrimSpace(account.Login)),
			AccountLogin:          account.Login,
			ProviderType:          account.ProviderType,
			Status:                account.Status,
			LastSyncAt:            account.LastSyncAt,
			Sends24h:              stat.sends,
			Replies24h:            stat.replies,
			Bounces24h:            stat.bounces,
			WaitingReply:          stat.waiting,
			ActiveBindings:        stat.bindings,
			RecommendedDailyCap:   dailyCap,
			RecommendedHourlyCap:  hourlyCap,
			RecommendedGapSeconds: gapSeconds,
			CollectorAccountID:    topology.collectorID,
			CollectorAccountLabel: topology.collectorLabel,
			ReplyTopology:         topology.replyTopology,
		})
	}
	util.WriteJSON(w, 200, map[string]any{"items": out})
}

func (h *Handlers) V2OutboundDomainDiagnostics(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	campaigns, err := h.svc.Store().ListOutboundCampaigns(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "outbound_domain_diagnostics_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	suppressions, err := h.svc.Store().ListOutboundSuppressions(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, 500, "outbound_domain_diagnostics_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	stats := map[string]*models.OutboundDomainDiagnostic{}
	for _, campaign := range campaigns {
		enrollments, err := h.svc.Store().ListOutboundEnrollmentsByCampaign(r.Context(), u.ID, campaign.ID)
		if err != nil {
			util.WriteError(w, 500, "outbound_domain_diagnostics_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		for _, item := range enrollments {
			domain := strings.ToLower(strings.TrimSpace(item.RecipientDomain))
			if domain == "" {
				continue
			}
			entry := stats[domain]
			if entry == nil {
				entry = &models.OutboundDomainDiagnostic{Domain: domain}
				stats[domain] = entry
			}
			switch item.Status {
			case "pending", "scheduled", "sending", "waiting_reply", "manual_only":
				entry.ActiveEnrollments++
			case "paused":
				entry.PausedEnrollments++
			}
			if item.ReplyOutcome != "" {
				entry.RepliedCount++
			}
			if !item.LastReplyAt.IsZero() && (entry.LastReplyAt.IsZero() || item.LastReplyAt.After(entry.LastReplyAt)) {
				entry.LastReplyAt = item.LastReplyAt
			}
		}
	}
	for _, item := range suppressions {
		if item.ScopeKind != "domain" {
			continue
		}
		domain := strings.ToLower(strings.TrimSpace(item.ScopeValue))
		entry := stats[domain]
		if entry == nil {
			entry = &models.OutboundDomainDiagnostic{Domain: domain}
			stats[domain] = entry
		}
		entry.Suppressed = true
		entry.SuppressionReason = firstNonEmpty(item.Reason, entry.SuppressionReason)
	}
	out := make([]models.OutboundDomainDiagnostic, 0, len(stats))
	for _, item := range stats {
		out = append(out, *item)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	util.WriteJSON(w, 200, map[string]any{"items": out})
}
