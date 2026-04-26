package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"despatch/internal/middleware"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/util"
)

const (
	replyFunnelReplyModeCollector = "collector"
	replyFunnelReplyModeSource    = "source"
	replyFunnelReplyModeSmart     = "smart"

	replyFunnelRoutingModeVirtual  = "virtual_inbox"
	replyFunnelRoutingModeManaged  = "managed_rules"
	replyFunnelRoutingModeAssisted = "assisted_forwarding"
)

type replyFunnelMutationRequest struct {
	Name               string   `json:"name"`
	SenderName         string   `json:"sender_name"`
	CollectorAccountID string   `json:"collector_account_id"`
	SourceAccountIDs   []string `json:"source_account_ids"`
	ReplyMode          string   `json:"reply_mode"`
	RoutingMode        string   `json:"routing_mode"`
	IncludeCollector   *bool    `json:"include_collector"`
	TargetReplyCount   int      `json:"target_reply_count"`
}

func normalizeReplyFunnelReplyMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case replyFunnelReplyModeSource:
		return replyFunnelReplyModeSource
	case replyFunnelReplyModeSmart:
		return replyFunnelReplyModeSmart
	default:
		return replyFunnelReplyModeCollector
	}
}

func normalizeReplyFunnelRoutingMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case replyFunnelRoutingModeManaged:
		return replyFunnelRoutingModeManaged
	case replyFunnelRoutingModeAssisted:
		return replyFunnelRoutingModeAssisted
	default:
		return replyFunnelRoutingModeVirtual
	}
}

func normalizeReplyFunnelAssistedForwardingState(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending":
		return "pending"
	case "in_progress":
		return "in_progress"
	case "confirmed":
		return "confirmed"
	case "needs_help":
		return "needs_help"
	default:
		return "not_required"
	}
}

func replyFunnelAccountLabel(account models.MailAccount) string {
	if label := strings.TrimSpace(account.DisplayName); label != "" {
		return label
	}
	if login := strings.TrimSpace(account.Login); login != "" {
		return login
	}
	return strings.TrimSpace(account.ID)
}

func replyFunnelDefaultSenderName(account models.MailAccount) string {
	if label := strings.TrimSpace(account.DisplayName); label != "" {
		return label
	}
	login := strings.TrimSpace(account.Login)
	if login == "" {
		return "Sender"
	}
	if idx := strings.Index(login, "@"); idx > 0 {
		login = login[:idx]
	}
	login = strings.ReplaceAll(login, ".", " ")
	login = strings.ReplaceAll(login, "_", " ")
	login = strings.ReplaceAll(login, "-", " ")
	login = strings.TrimSpace(login)
	if login == "" {
		return "Sender"
	}
	return login
}

func normalizeReplyFunnelSourceAccountIDs(raw []string, collectorAccountID string) []string {
	collectorAccountID = strings.TrimSpace(collectorAccountID)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		id := strings.TrimSpace(item)
		if id == "" || id == collectorAccountID {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func replyFunnelSavedSearchName(funnelName string) string {
	name := strings.TrimSpace(funnelName)
	if name == "" {
		name = "Funnel"
	}
	return "Funnel · " + name
}

func replyFunnelSavedSearchJSON(accountIDs []string) (string, error) {
	payload := map[string]any{
		"account_scope": "all",
		"view_kind":     "mailbox",
		"mailbox":       "Inbox",
		"smart_view":    "",
		"query":         "",
		"filters": map[string]any{
			"from":            "",
			"to":              "",
			"subject":         "",
			"date_from":       "",
			"date_to":         "",
			"unread":          false,
			"flagged":         false,
			"has_attachments": false,
			"waiting":         false,
			"snoozed":         false,
			"follow_up":       false,
			"category_id":     "",
			"tag_ids":         []string{},
			"account_ids":     accountIDs,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func replyFunnelRowKey(role, accountID string) string {
	return strings.ToLower(strings.TrimSpace(role)) + ":" + strings.TrimSpace(accountID)
}

func appendReplyFunnelError(current, message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return strings.TrimSpace(current)
	}
	current = strings.TrimSpace(current)
	if current == "" {
		return message
	}
	if strings.Contains(current, message) {
		return current
	}
	return current + "; " + message
}

func (h *Handlers) replyFunnelAccountsByID(ctx context.Context, userID string) (map[string]models.MailAccount, error) {
	items, err := h.svc.Store().ListMailAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]models.MailAccount, len(items))
	for _, item := range items {
		item = h.decorateMailAccount(item)
		out[item.ID] = item
	}
	return out, nil
}

func replyFunnelAssistedForwardingWarning(source, collector models.MailAccount) string {
	source = source
	switch service.NormalizeMailProviderType(source.ProviderType) {
	case service.MailProviderTypeLibero:
		if service.NormalizeMailProviderType(collector.ProviderType) == service.MailProviderTypeLibero {
			return ""
		}
		return "Libero documents full automatic forwarding to non-Libero addresses as a Mail Plus capability."
	case service.MailProviderTypeGmail:
		if service.GmailMailboxKind(source.Login) == "workspace" {
			return "Google Workspace users can forward to one verified address per mailbox, and admins may need to allow automatic forwarding before the collector address can be used."
		}
		return "Gmail forwards only new mail and skips spam. The collector address must be verified before forwarding can be turned on."
	default:
		return ""
	}
}

func replyFunnelAssistedForwardingURL(account models.MailAccount) string {
	if service.NormalizeMailProviderType(account.ProviderType) == service.MailProviderTypeGmail && service.GmailMailboxKind(account.Login) == "workspace" {
		if href := strings.TrimSpace(account.HelperLinks["workspace_forwarding"]); href != "" {
			return href
		}
	}
	return strings.TrimSpace(account.HelperLinks["forwarding"])
}

func (h *Handlers) ensureReplyFunnelSavedSearch(ctx context.Context, u models.User, funnel models.ReplyFunnel, includedAccountIDs []string) (string, error) {
	filtersJSON, err := replyFunnelSavedSearchJSON(includedAccountIDs)
	if err != nil {
		return "", err
	}
	target := models.SavedSearch{
		UserID:      u.ID,
		AccountID:   "",
		Name:        replyFunnelSavedSearchName(funnel.Name),
		FiltersJSON: filtersJSON,
		Pinned:      true,
	}
	if savedSearchID := strings.TrimSpace(funnel.SavedSearchID); savedSearchID != "" {
		target.ID = savedSearchID
		if _, err := h.svc.Store().UpdateSavedSearch(ctx, target); err == nil {
			return savedSearchID, nil
		} else if err != store.ErrNotFound {
			return "", err
		}
	}
	created, err := h.svc.Store().CreateSavedSearch(ctx, target)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

func (h *Handlers) ensureReplyFunnelIdentity(ctx context.Context, account models.MailAccount, senderName string, row models.ReplyFunnelAccount, cache map[string][]models.MailIdentity) (models.ReplyFunnelAccount, error) {
	senderName = strings.TrimSpace(senderName)
	if senderName == "" {
		senderName = replyFunnelDefaultSenderName(account)
	}
	loadIdentities := func() ([]models.MailIdentity, error) {
		if items, ok := cache[account.ID]; ok {
			return items, nil
		}
		items, err := h.svc.Store().ListMailIdentities(ctx, account.ID)
		if err != nil {
			return nil, err
		}
		cache[account.ID] = items
		return items, nil
	}
	var identity models.MailIdentity
	if identityID := strings.TrimSpace(row.SenderIdentityID); identityID != "" {
		item, err := h.svc.Store().GetMailIdentityByID(ctx, identityID)
		if err == nil && item.AccountID == account.ID {
			identity = item
		} else if err != nil && err != store.ErrNotFound {
			return row, err
		}
	}
	if strings.TrimSpace(identity.ID) == "" {
		items, err := loadIdentities()
		if err != nil {
			return row, err
		}
		for _, item := range items {
			if strings.EqualFold(strings.TrimSpace(item.FromEmail), strings.TrimSpace(account.Login)) &&
				strings.EqualFold(strings.TrimSpace(item.DisplayName), senderName) {
				identity = item
				break
			}
		}
	}
	if strings.TrimSpace(identity.ID) == "" {
		created, err := h.svc.Store().CreateMailIdentity(ctx, models.MailIdentity{
			AccountID:   account.ID,
			DisplayName: senderName,
			FromEmail:   strings.TrimSpace(account.Login),
			IsDefault:   false,
		})
		if err != nil {
			return row, err
		}
		items := append(cache[account.ID], created)
		cache[account.ID] = items
		identity = created
	} else if strings.TrimSpace(identity.DisplayName) != senderName || !strings.EqualFold(strings.TrimSpace(identity.FromEmail), strings.TrimSpace(account.Login)) {
		identity.DisplayName = senderName
		identity.FromEmail = strings.TrimSpace(account.Login)
		updated, err := h.svc.Store().UpdateMailIdentity(ctx, identity)
		if err != nil {
			return row, err
		}
		identity = updated
		items, _ := loadIdentities()
		for i := range items {
			if items[i].ID == identity.ID {
				items[i] = identity
			}
		}
		cache[account.ID] = items
	}
	row.SenderIdentityID = identity.ID
	row.LastApplyError = ""
	return row, nil
}

func replyFunnelRedirectRuleName(funnel models.ReplyFunnel) string {
	name := strings.TrimSpace(funnel.Name)
	if name == "" {
		name = "Collector"
	}
	return "Despatch Funnel · " + name
}

func (h *Handlers) ensureReplyFunnelRedirectRule(ctx context.Context, account models.MailAccount, collector models.MailAccount, funnel models.ReplyFunnel, row models.ReplyFunnelAccount, cache map[string][]models.MailRule) (models.ReplyFunnelAccount, bool, error) {
	loadRules := func() ([]models.MailRule, error) {
		if items, ok := cache[account.ID]; ok {
			return items, nil
		}
		items, err := h.svc.Store().ListMailRules(ctx, account.ID)
		if err != nil {
			return nil, err
		}
		cache[account.ID] = items
		return items, nil
	}
	var rule models.MailRule
	ruleFound := false
	if ruleID := strings.TrimSpace(row.RedirectRuleID); ruleID != "" {
		item, err := h.svc.Store().GetMailRuleByID(ctx, account.ID, ruleID)
		if err == nil {
			rule = item
			ruleFound = true
		} else if err != nil && err != store.ErrNotFound {
			return row, false, err
		}
	}
	if !ruleFound {
		items, err := loadRules()
		if err != nil {
			return row, false, err
		}
		expectedName := replyFunnelRedirectRuleName(funnel)
		expectedTo := strings.TrimSpace(account.Login)
		expectedRedirect := strings.TrimSpace(collector.Login)
		for _, item := range items {
			if strings.TrimSpace(item.Name) != expectedName {
				continue
			}
			if strings.TrimSpace(item.Conditions.ToContains) != expectedTo {
				continue
			}
			if strings.TrimSpace(item.Actions.Redirect) != expectedRedirect {
				continue
			}
			rule = item
			ruleFound = true
			break
		}
	}
	expected := models.MailRule{
		ID:        strings.TrimSpace(rule.ID),
		AccountID: account.ID,
		Name:      replyFunnelRedirectRuleName(funnel),
		Enabled:   true,
		Position:  rule.Position,
		MatchMode: "all",
		Conditions: models.MailRuleConditions{
			ToContains: strings.TrimSpace(account.Login),
		},
		Actions: models.MailRuleActions{
			Redirect: strings.TrimSpace(collector.Login),
			Stop:     true,
		},
	}
	if !ruleFound {
		created, err := h.svc.Store().CreateMailRule(ctx, expected)
		if err != nil {
			return row, false, err
		}
		items := append(cache[account.ID], created)
		cache[account.ID] = mailRulesSortedByPosition(items)
		row.RedirectRuleID = created.ID
		row.LastApplyError = ""
		return row, true, nil
	}
	ruleChanged := strings.TrimSpace(rule.Name) != expected.Name ||
		!rule.Enabled ||
		strings.TrimSpace(rule.MatchMode) != expected.MatchMode ||
		strings.TrimSpace(rule.Conditions.ToContains) != expected.Conditions.ToContains ||
		strings.TrimSpace(rule.Actions.Redirect) != expected.Actions.Redirect ||
		!rule.Actions.Stop ||
		rule.Actions.MarkRead ||
		strings.TrimSpace(rule.Actions.MoveToMailbox) != "" ||
		strings.TrimSpace(rule.Actions.MoveToRole) != ""
	if ruleChanged {
		rule.Name = expected.Name
		rule.Enabled = true
		rule.MatchMode = "all"
		rule.Conditions = expected.Conditions
		rule.Actions = expected.Actions
		updated, err := h.svc.Store().UpdateMailRule(ctx, rule)
		if err != nil {
			return row, false, err
		}
		items, _ := loadRules()
		for i := range items {
			if items[i].ID == updated.ID {
				items[i] = updated
			}
		}
		cache[account.ID] = mailRulesSortedByPosition(items)
		rule = updated
	}
	row.RedirectRuleID = rule.ID
	row.LastApplyError = ""
	return row, ruleChanged, nil
}

func (h *Handlers) deleteReplyFunnelRedirectRule(ctx context.Context, accountID string, row models.ReplyFunnelAccount, cache map[string][]models.MailRule) (models.ReplyFunnelAccount, bool, error) {
	ruleID := strings.TrimSpace(row.RedirectRuleID)
	if ruleID == "" {
		row.LastApplyError = ""
		return row, false, nil
	}
	err := h.svc.Store().DeleteMailRule(ctx, accountID, ruleID)
	if err != nil && err != store.ErrNotFound {
		return row, false, err
	}
	if items, ok := cache[accountID]; ok {
		filtered := items[:0]
		for _, item := range items {
			if item.ID == ruleID {
				continue
			}
			filtered = append(filtered, item)
		}
		cache[accountID] = filtered
	}
	row.RedirectRuleID = ""
	row.LastApplyError = ""
	return row, true, nil
}

func (h *Handlers) syncReplyFunnelRuleScripts(ctx context.Context, u models.User, accountIDs []string, rows map[string]models.ReplyFunnelAccount) map[string]models.ReplyFunnelAccount {
	seen := map[string]struct{}{}
	for _, accountID := range accountIDs {
		accountID = strings.TrimSpace(accountID)
		if accountID == "" {
			continue
		}
		if _, ok := seen[accountID]; ok {
			continue
		}
		seen[accountID] = struct{}{}
		account, err := h.svc.Store().GetMailAccountByID(ctx, u.ID, accountID)
		if err != nil {
			continue
		}
		mailboxes, mailboxErr := h.listAccountMailboxesWithRolesBestEffort(ctx, account)
		if mailboxErr != nil {
			mailboxes = nil
		}
		if err := h.syncManagedRulesScript(ctx, account, mailboxes); err != nil {
			key := replyFunnelRowKey("source", accountID)
			row := rows[key]
			row.LastApplyError = appendReplyFunnelError(row.LastApplyError, err.Error())
			if updated, updateErr := h.svc.Store().UpsertReplyFunnelAccount(ctx, row); updateErr == nil {
				rows[key] = updated
			} else {
				rows[key] = row
			}
		}
	}
	return rows
}

func (h *Handlers) buildReplyFunnelResponse(ctx context.Context, u models.User, funnel models.ReplyFunnel) (models.ReplyFunnel, error) {
	rows, err := h.svc.Store().ListReplyFunnelAccounts(ctx, funnel.ID)
	if err != nil {
		return models.ReplyFunnel{}, err
	}
	accountsByID, err := h.replyFunnelAccountsByID(ctx, u.ID)
	if err != nil {
		return models.ReplyFunnel{}, err
	}
	sourceIDs := make([]string, 0, len(rows))
	collectorAccount := accountsByID[funnel.CollectorAccountID]
	for i := range rows {
		if account, ok := accountsByID[rows[i].AccountID]; ok {
			rows[i].AccountLabel = replyFunnelAccountLabel(account)
			rows[i].AccountLogin = strings.TrimSpace(account.Login)
			rows[i].ProviderType = service.NormalizeMailProviderType(account.ProviderType)
			rows[i].ProviderLabel = strings.TrimSpace(account.ProviderLabel)
			if rows[i].Role == "source" && funnel.RoutingMode == replyFunnelRoutingModeAssisted && service.MailProviderSupportsAssistedForwarding(account.ProviderType) {
				rows[i].AssistedForwardingURL = replyFunnelAssistedForwardingURL(account)
				rows[i].AssistedForwardingWarning = replyFunnelAssistedForwardingWarning(account, collectorAccount)
			}
		}
		if rows[i].Role == "source" {
			sourceIDs = append(sourceIDs, rows[i].AccountID)
		}
	}
	if collector, ok := accountsByID[funnel.CollectorAccountID]; ok {
		funnel.CollectorLabel = replyFunnelAccountLabel(collector)
		funnel.CollectorLogin = strings.TrimSpace(collector.Login)
	}
	funnel.Accounts = rows
	funnel.SourceAccountIDs = sourceIDs
	return funnel, nil
}

func (h *Handlers) V2ListReplyFunnels(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, err := h.svc.Store().ListReplyFunnels(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "funnels_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out := make([]models.ReplyFunnel, 0, len(items))
	for _, item := range items {
		detailed, err := h.buildReplyFunnelResponse(r.Context(), u, item)
		if err != nil {
			util.WriteError(w, http.StatusInternalServerError, "funnels_list_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		out = append(out, detailed)
	}
	util.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handlers) V2GetReplyFunnel(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	funnel, err := h.svc.Store().GetReplyFunnelByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "funnel_not_found", "funnel not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "funnel_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out, err := h.buildReplyFunnelResponse(r.Context(), u, funnel)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "funnel_get_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) upsertReplyFunnel(ctx context.Context, u models.User, existingID string, req replyFunnelMutationRequest) (models.ReplyFunnel, error) {
	accountsByID, err := h.replyFunnelAccountsByID(ctx, u.ID)
	if err != nil {
		return models.ReplyFunnel{}, err
	}
	collectorAccountID := strings.TrimSpace(req.CollectorAccountID)
	collectorAccount, ok := accountsByID[collectorAccountID]
	if !ok {
		return models.ReplyFunnel{}, fmt.Errorf("collector account is required")
	}
	sourceAccountIDs := normalizeReplyFunnelSourceAccountIDs(req.SourceAccountIDs, collectorAccountID)
	if len(sourceAccountIDs) == 0 {
		return models.ReplyFunnel{}, fmt.Errorf("choose at least one source account")
	}
	for _, accountID := range sourceAccountIDs {
		if _, ok := accountsByID[accountID]; !ok {
			return models.ReplyFunnel{}, fmt.Errorf("source account %q is unavailable", accountID)
		}
	}
	routingMode := normalizeReplyFunnelRoutingMode(req.RoutingMode)
	if routingMode == replyFunnelRoutingModeAssisted {
		for _, accountID := range sourceAccountIDs {
			account := accountsByID[accountID]
			if !service.MailProviderSupportsAssistedForwarding(account.ProviderType) {
				return models.ReplyFunnel{}, fmt.Errorf("%s does not support assisted forwarding in Despatch yet", replyFunnelAccountLabel(account))
			}
		}
	}
	includeCollector := true
	if req.IncludeCollector != nil {
		includeCollector = *req.IncludeCollector
	}
	funnelName := strings.TrimSpace(req.Name)
	if funnelName == "" {
		return models.ReplyFunnel{}, fmt.Errorf("funnel name is required")
	}
	senderName := strings.TrimSpace(req.SenderName)
	if senderName == "" {
		senderName = replyFunnelDefaultSenderName(collectorAccount)
	}
	targetReplyCount := req.TargetReplyCount
	if targetReplyCount <= 0 {
		targetReplyCount = 100
	}
	var funnel models.ReplyFunnel
	var existingRows []models.ReplyFunnelAccount
	if strings.TrimSpace(existingID) != "" {
		funnel, err = h.svc.Store().GetReplyFunnelByID(ctx, u.ID, existingID)
		if err != nil {
			return models.ReplyFunnel{}, err
		}
		existingRows, err = h.svc.Store().ListReplyFunnelAccounts(ctx, existingID)
		if err != nil {
			return models.ReplyFunnel{}, err
		}
		funnel.Name = funnelName
		funnel.SenderName = senderName
		funnel.CollectorAccountID = collectorAccountID
		funnel.ReplyMode = normalizeReplyFunnelReplyMode(req.ReplyMode)
		funnel.RoutingMode = routingMode
		funnel.IncludeCollector = includeCollector
		funnel.TargetReplyCount = targetReplyCount
		funnel, err = h.svc.Store().UpdateReplyFunnel(ctx, funnel)
		if err != nil {
			return models.ReplyFunnel{}, err
		}
	} else {
		funnel = models.ReplyFunnel{
			UserID:             u.ID,
			Name:               funnelName,
			SenderName:         senderName,
			CollectorAccountID: collectorAccountID,
			ReplyMode:          normalizeReplyFunnelReplyMode(req.ReplyMode),
			RoutingMode:        routingMode,
			IncludeCollector:   includeCollector,
			TargetReplyCount:   targetReplyCount,
		}
		funnel, err = h.svc.Store().CreateReplyFunnel(ctx, funnel)
		if err != nil {
			return models.ReplyFunnel{}, err
		}
	}
	existingByKey := map[string]models.ReplyFunnelAccount{}
	for _, row := range existingRows {
		existingByKey[replyFunnelRowKey(row.Role, row.AccountID)] = row
	}
	targetKeys := map[string]struct{}{}
	upsertedRows := map[string]models.ReplyFunnelAccount{}
	collectorKey := replyFunnelRowKey("collector", collectorAccountID)
	targetKeys[collectorKey] = struct{}{}
	collectorRow := existingByKey[collectorKey]
	collectorRow.FunnelID = funnel.ID
	collectorRow.AccountID = collectorAccountID
	collectorRow.Role = "collector"
	collectorRow.Position = 0
	collectorRow.LastApplyError = ""
	collectorRow.AssistedForwardingState = "not_required"
	collectorRow.AssistedForwardingNotes = ""
	collectorRow.AssistedForwardingCheckedAt = time.Time{}
	collectorRow.AssistedForwardingConfirmed = time.Time{}
	collectorRow, err = h.svc.Store().UpsertReplyFunnelAccount(ctx, collectorRow)
	if err != nil {
		return models.ReplyFunnel{}, err
	}
	upsertedRows[collectorKey] = collectorRow

	for idx, accountID := range sourceAccountIDs {
		key := replyFunnelRowKey("source", accountID)
		targetKeys[key] = struct{}{}
		row := existingByKey[key]
		row.FunnelID = funnel.ID
		row.AccountID = accountID
		row.Role = "source"
		row.Position = idx
		row.LastApplyError = ""
		if routingMode == replyFunnelRoutingModeAssisted {
			row.AssistedForwardingState = normalizeReplyFunnelAssistedForwardingState(row.AssistedForwardingState)
			if row.AssistedForwardingState == "not_required" {
				row.AssistedForwardingState = "pending"
			}
		} else {
			row.AssistedForwardingState = "not_required"
			row.AssistedForwardingNotes = ""
			row.AssistedForwardingCheckedAt = time.Time{}
			row.AssistedForwardingConfirmed = time.Time{}
		}
		row, err = h.svc.Store().UpsertReplyFunnelAccount(ctx, row)
		if err != nil {
			return models.ReplyFunnel{}, err
		}
		upsertedRows[key] = row
	}

	ruleCache := map[string][]models.MailRule{}
	affectedRuleAccountIDs := make([]string, 0, len(existingRows))
	for _, row := range existingRows {
		key := replyFunnelRowKey(row.Role, row.AccountID)
		if _, ok := targetKeys[key]; ok {
			continue
		}
		if row.Role == "source" && strings.TrimSpace(row.RedirectRuleID) != "" {
			if _, _, err := h.deleteReplyFunnelRedirectRule(ctx, row.AccountID, row, ruleCache); err == nil || err == store.ErrNotFound {
				affectedRuleAccountIDs = append(affectedRuleAccountIDs, row.AccountID)
			}
		}
		if err := h.svc.Store().DeleteReplyFunnelAccountByID(ctx, row.ID); err != nil && err != store.ErrNotFound {
			return models.ReplyFunnel{}, err
		}
	}

	includedAccountIDs := append([]string(nil), sourceAccountIDs...)
	if includeCollector {
		includedAccountIDs = append(includedAccountIDs, collectorAccountID)
	}
	savedSearchID, err := h.ensureReplyFunnelSavedSearch(ctx, u, funnel, includedAccountIDs)
	if err != nil {
		return models.ReplyFunnel{}, err
	}
	if strings.TrimSpace(savedSearchID) != strings.TrimSpace(funnel.SavedSearchID) {
		funnel.SavedSearchID = savedSearchID
		funnel, err = h.svc.Store().UpdateReplyFunnel(ctx, funnel)
		if err != nil {
			return models.ReplyFunnel{}, err
		}
	}

	identityCache := map[string][]models.MailIdentity{}
	for key, row := range upsertedRows {
		account, ok := accountsByID[row.AccountID]
		if !ok {
			row.LastApplyError = appendReplyFunnelError(row.LastApplyError, "account unavailable")
			updated, _ := h.svc.Store().UpsertReplyFunnelAccount(ctx, row)
			upsertedRows[key] = updated
			continue
		}
		updatedRow, err := h.ensureReplyFunnelIdentity(ctx, account, senderName, row, identityCache)
		if err != nil {
			row.LastApplyError = appendReplyFunnelError(row.LastApplyError, err.Error())
			updatedRow, _ = h.svc.Store().UpsertReplyFunnelAccount(ctx, row)
			upsertedRows[key] = updatedRow
			continue
		}
		updatedRow, err = h.svc.Store().UpsertReplyFunnelAccount(ctx, updatedRow)
		if err != nil {
			return models.ReplyFunnel{}, err
		}
		upsertedRows[key] = updatedRow
	}

	for _, accountID := range sourceAccountIDs {
		key := replyFunnelRowKey("source", accountID)
		row := upsertedRows[key]
		account := accountsByID[accountID]
		if funnel.RoutingMode == replyFunnelRoutingModeManaged {
			updatedRow, changed, err := h.ensureReplyFunnelRedirectRule(ctx, account, collectorAccount, funnel, row, ruleCache)
			if err != nil {
				row.LastApplyError = appendReplyFunnelError(row.LastApplyError, err.Error())
				updatedRow, _ = h.svc.Store().UpsertReplyFunnelAccount(ctx, row)
				upsertedRows[key] = updatedRow
				continue
			}
			updatedRow, err = h.svc.Store().UpsertReplyFunnelAccount(ctx, updatedRow)
			if err != nil {
				return models.ReplyFunnel{}, err
			}
			upsertedRows[key] = updatedRow
			if changed {
				affectedRuleAccountIDs = append(affectedRuleAccountIDs, accountID)
			}
		} else {
			updatedRow, changed, err := h.deleteReplyFunnelRedirectRule(ctx, accountID, row, ruleCache)
			if err != nil {
				row.LastApplyError = appendReplyFunnelError(row.LastApplyError, err.Error())
				updatedRow, _ = h.svc.Store().UpsertReplyFunnelAccount(ctx, row)
				upsertedRows[key] = updatedRow
				continue
			}
			if funnel.RoutingMode == replyFunnelRoutingModeAssisted {
				updatedRow.AssistedForwardingState = normalizeReplyFunnelAssistedForwardingState(updatedRow.AssistedForwardingState)
				if updatedRow.AssistedForwardingState == "not_required" {
					updatedRow.AssistedForwardingState = "pending"
				}
			} else {
				updatedRow.AssistedForwardingState = "not_required"
				updatedRow.AssistedForwardingNotes = ""
				updatedRow.AssistedForwardingCheckedAt = time.Time{}
				updatedRow.AssistedForwardingConfirmed = time.Time{}
			}
			updatedRow, err = h.svc.Store().UpsertReplyFunnelAccount(ctx, updatedRow)
			if err != nil {
				return models.ReplyFunnel{}, err
			}
			upsertedRows[key] = updatedRow
			if changed {
				affectedRuleAccountIDs = append(affectedRuleAccountIDs, accountID)
			}
		}
	}

	if len(affectedRuleAccountIDs) > 0 {
		upsertedRows = h.syncReplyFunnelRuleScripts(ctx, u, affectedRuleAccountIDs, upsertedRows)
	}

	return h.buildReplyFunnelResponse(ctx, u, funnel)
}

func (h *Handlers) V2CreateReplyFunnel(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req replyFunnelMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	out, err := h.upsertReplyFunnel(r.Context(), u, "", req)
	if err != nil {
		util.WriteError(w, http.StatusBadRequest, "funnel_create_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, http.StatusCreated, out)
}

func (h *Handlers) V2UpdateReplyFunnel(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req replyFunnelMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	out, err := h.upsertReplyFunnel(r.Context(), u, chi.URLParam(r, "id"), req)
	if err != nil {
		status := http.StatusBadRequest
		code := "funnel_update_failed"
		if err == store.ErrNotFound {
			status = http.StatusNotFound
			code = "funnel_not_found"
		}
		util.WriteError(w, status, code, err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) V2UpdateReplyFunnelAssistedForwarding(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	funnel, err := h.svc.Store().GetReplyFunnelByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "funnel_not_found", "funnel not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "funnel_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if normalizeReplyFunnelRoutingMode(funnel.RoutingMode) != replyFunnelRoutingModeAssisted {
		util.WriteError(w, http.StatusBadRequest, "assisted_forwarding_unavailable", "this funnel is not using assisted forwarding", middleware.RequestID(r.Context()))
		return
	}
	row, err := h.svc.Store().GetReplyFunnelAccountByKey(r.Context(), funnel.ID, chi.URLParam(r, "account_id"), "source")
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "funnel_account_not_found", "source account not found in this funnel", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "funnel_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req struct {
		State string `json:"state"`
		Notes string `json:"notes"`
	}
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	state := normalizeReplyFunnelAssistedForwardingState(req.State)
	now := time.Now().UTC()
	row.AssistedForwardingState = state
	row.AssistedForwardingNotes = strings.TrimSpace(req.Notes)
	switch state {
	case "confirmed":
		row.AssistedForwardingCheckedAt = now
		row.AssistedForwardingConfirmed = now
	case "pending":
		row.AssistedForwardingCheckedAt = time.Time{}
		row.AssistedForwardingConfirmed = time.Time{}
	case "in_progress", "needs_help":
		row.AssistedForwardingCheckedAt = now
		row.AssistedForwardingConfirmed = time.Time{}
	default:
		row.AssistedForwardingNotes = ""
		row.AssistedForwardingCheckedAt = time.Time{}
		row.AssistedForwardingConfirmed = time.Time{}
	}
	if _, err := h.svc.Store().UpsertReplyFunnelAccount(r.Context(), row); err != nil {
		util.WriteError(w, http.StatusInternalServerError, "funnel_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	out, err := h.buildReplyFunnelResponse(r.Context(), u, funnel)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "funnel_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) V2DeleteReplyFunnel(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	funnel, err := h.svc.Store().GetReplyFunnelByID(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "funnel_not_found", "funnel not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "funnel_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	rows, err := h.svc.Store().ListReplyFunnelAccounts(r.Context(), funnel.ID)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "funnel_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	ruleCache := map[string][]models.MailRule{}
	affectedRuleAccountIDs := make([]string, 0, len(rows))
	warnings := make([]string, 0, 4)
	for _, row := range rows {
		if row.Role != "source" || strings.TrimSpace(row.RedirectRuleID) == "" {
			continue
		}
		if _, changed, err := h.deleteReplyFunnelRedirectRule(r.Context(), row.AccountID, row, ruleCache); err != nil {
			warnings = append(warnings, err.Error())
		} else if changed {
			affectedRuleAccountIDs = append(affectedRuleAccountIDs, row.AccountID)
		}
	}
	if strings.TrimSpace(funnel.SavedSearchID) != "" {
		if err := h.svc.Store().DeleteSavedSearch(r.Context(), u.ID, funnel.SavedSearchID); err != nil && err != store.ErrNotFound {
			warnings = append(warnings, err.Error())
		}
	}
	if err := h.svc.Store().DeleteReplyFunnel(r.Context(), u.ID, funnel.ID); err != nil {
		util.WriteError(w, http.StatusInternalServerError, "funnel_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if len(affectedRuleAccountIDs) > 0 {
		_ = h.syncReplyFunnelRuleScripts(r.Context(), u, affectedRuleAccountIDs, map[string]models.ReplyFunnelAccount{})
	}
	util.WriteJSON(w, http.StatusOK, map[string]any{
		"status":   "deleted",
		"id":       funnel.ID,
		"warnings": warnings,
	})
}
