package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	stdmail "net/mail"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"despatch/internal/mail"
	"despatch/internal/middleware"
	"despatch/internal/models"
	"despatch/internal/store"
	"despatch/internal/util"
)

const managedRulesScriptName = "despatch-managed-rules"

type mailRuleMutationRequest struct {
	Name       string                    `json:"name"`
	Enabled    *bool                     `json:"enabled"`
	Position   *int                      `json:"position"`
	MatchMode  string                    `json:"match_mode"`
	Conditions models.MailRuleConditions `json:"conditions"`
	Actions    models.MailRuleActions    `json:"actions"`
}

type mailRuleReorderRequest struct {
	IDs []string `json:"ids"`
}

func trimMailRuleConditions(in models.MailRuleConditions) models.MailRuleConditions {
	return models.MailRuleConditions{
		FromContains:    strings.TrimSpace(in.FromContains),
		FromDomainIs:    strings.ToLower(strings.Trim(strings.TrimSpace(in.FromDomainIs), ".")),
		ToContains:      strings.TrimSpace(in.ToContains),
		SubjectContains: strings.TrimSpace(in.SubjectContains),
		BodyContains:    strings.TrimSpace(in.BodyContains),
	}
}

func trimMailRuleActions(in models.MailRuleActions) models.MailRuleActions {
	return models.MailRuleActions{
		MoveToMailbox: strings.TrimSpace(in.MoveToMailbox),
		MoveToRole:    normalizeAggregateMailboxRole(strings.TrimSpace(in.MoveToRole), strings.TrimSpace(in.MoveToRole)),
		MarkRead:      in.MarkRead,
		Redirect:      strings.TrimSpace(in.Redirect),
		Stop:          in.Stop,
	}
}

func mailRuleHasConditions(in models.MailRuleConditions) bool {
	return strings.TrimSpace(in.FromContains) != "" ||
		strings.TrimSpace(in.FromDomainIs) != "" ||
		strings.TrimSpace(in.ToContains) != "" ||
		strings.TrimSpace(in.SubjectContains) != "" ||
		strings.TrimSpace(in.BodyContains) != ""
}

func mailRuleHasActions(in models.MailRuleActions) bool {
	return strings.TrimSpace(in.MoveToMailbox) != "" ||
		strings.TrimSpace(in.MoveToRole) != "" ||
		in.MarkRead ||
		strings.TrimSpace(in.Redirect) != "" ||
		in.Stop
}

func validateMailRule(req mailRuleMutationRequest) error {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return fmt.Errorf("rule name is required")
	}
	conditions := trimMailRuleConditions(req.Conditions)
	actions := trimMailRuleActions(req.Actions)
	if !mailRuleHasConditions(conditions) {
		return fmt.Errorf("at least one condition is required")
	}
	if !mailRuleHasActions(actions) {
		return fmt.Errorf("at least one action is required")
	}
	if strings.TrimSpace(actions.MoveToMailbox) != "" && strings.TrimSpace(actions.MoveToRole) != "" {
		return fmt.Errorf("choose either move_to_mailbox or move_to_role")
	}
	if actions.MoveToRole != "" && actions.MoveToRole != "junk" && actions.MoveToRole != "trash" {
		return fmt.Errorf("unsupported move_to_role")
	}
	if actions.Redirect != "" {
		if _, err := stdmail.ParseAddress(actions.Redirect); err != nil {
			return fmt.Errorf("redirect must be a valid email address")
		}
	}
	return nil
}

func sieveEscape(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(value)
}

func ruleMailboxByRole(mailboxes []mail.Mailbox, role string) string {
	target := normalizeAggregateMailboxRole(role, role)
	if target == "" {
		return ""
	}
	for _, item := range mailboxes {
		if normalizeAggregateMailboxRole(item.Role, item.Name) == target {
			return strings.TrimSpace(item.Name)
		}
	}
	return ""
}

func compileMailRuleConditionFragments(rule models.MailRule) []string {
	conditions := trimMailRuleConditions(rule.Conditions)
	out := make([]string, 0, 5)
	if conditions.FromContains != "" {
		out = append(out, fmt.Sprintf(`header :contains "from" "%s"`, sieveEscape(conditions.FromContains)))
	}
	if conditions.FromDomainIs != "" {
		out = append(out, fmt.Sprintf(`address :domain :is "from" "%s"`, sieveEscape(conditions.FromDomainIs)))
	}
	if conditions.ToContains != "" {
		out = append(out, fmt.Sprintf(`address :contains ["to", "cc", "bcc"] "%s"`, sieveEscape(conditions.ToContains)))
	}
	if conditions.SubjectContains != "" {
		out = append(out, fmt.Sprintf(`header :contains "subject" "%s"`, sieveEscape(conditions.SubjectContains)))
	}
	if conditions.BodyContains != "" {
		out = append(out, fmt.Sprintf(`body :contains "%s"`, sieveEscape(conditions.BodyContains)))
	}
	return out
}

func compileMailRuleActionFragments(rule models.MailRule, mailboxes []mail.Mailbox) ([]string, error) {
	actions := trimMailRuleActions(rule.Actions)
	out := make([]string, 0, 4)
	targetMailbox := strings.TrimSpace(actions.MoveToMailbox)
	if targetMailbox == "" && actions.MoveToRole != "" {
		targetMailbox = ruleMailboxByRole(mailboxes, actions.MoveToRole)
		if targetMailbox == "" {
			if actions.MoveToRole == "junk" || actions.MoveToRole == "trash" {
				return nil, specialMailboxRequiredError{Role: actions.MoveToRole}
			}
			return nil, fmt.Errorf("%s mailbox is unavailable", aggregateMailboxDisplayName(actions.MoveToRole))
		}
	}
	if targetMailbox != "" {
		out = append(out, fmt.Sprintf(`fileinto "%s";`, sieveEscape(targetMailbox)))
	}
	if actions.MarkRead {
		out = append(out, `addflag "\\Seen";`)
	}
	if actions.Redirect != "" {
		out = append(out, fmt.Sprintf(`redirect "%s";`, sieveEscape(actions.Redirect)))
	}
	if actions.Stop {
		out = append(out, `stop;`)
	}
	return out, nil
}

func compileManagedMailRules(rules []models.MailRule, mailboxes []mail.Mailbox) (string, error) {
	requiresOrder := []string{"fileinto", "imap4flags", "redirect", "body"}
	requiresSet := map[string]struct{}{}
	blocks := make([]string, 0, len(rules))
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		conditions := compileMailRuleConditionFragments(rule)
		if len(conditions) == 0 {
			continue
		}
		actions, err := compileMailRuleActionFragments(rule, mailboxes)
		if err != nil {
			return "", err
		}
		if len(actions) == 0 {
			continue
		}
		if trimMailRuleActions(rule.Actions).MoveToMailbox != "" || trimMailRuleActions(rule.Actions).MoveToRole != "" {
			requiresSet["fileinto"] = struct{}{}
		}
		if trimMailRuleActions(rule.Actions).MarkRead {
			requiresSet["imap4flags"] = struct{}{}
		}
		if trimMailRuleActions(rule.Actions).Redirect != "" {
			requiresSet["redirect"] = struct{}{}
		}
		if trimMailRuleConditions(rule.Conditions).BodyContains != "" {
			requiresSet["body"] = struct{}{}
		}
		conditionExpr := conditions[0]
		if len(conditions) > 1 {
			joiner := "allof"
			if strings.EqualFold(strings.TrimSpace(rule.MatchMode), "any") {
				joiner = "anyof"
			}
			conditionExpr = fmt.Sprintf("%s(%s)", joiner, strings.Join(conditions, ", "))
		}
		block := fmt.Sprintf("# %s\nif %s {\n  %s\n}", sieveEscape(strings.TrimSpace(rule.Name)), conditionExpr, strings.Join(actions, "\n  "))
		blocks = append(blocks, block)
	}
	requires := make([]string, 0, len(requiresSet))
	for _, item := range requiresOrder {
		if _, ok := requiresSet[item]; ok {
			requires = append(requires, item)
		}
	}
	if len(requires) == 0 {
		requires = append(requires, "fileinto")
	}
	requireLiterals := make([]string, 0, len(requires))
	for _, item := range requires {
		requireLiterals = append(requireLiterals, fmt.Sprintf(`"%s"`, item))
	}
	lines := []string{
		`require [` + strings.Join(requireLiterals, ", ") + `];`,
		"",
		"# Managed by Despatch. Edit structured rules in Settings > Mail > Rules & Junk.",
	}
	if len(blocks) == 0 {
		lines = append(lines, "# No enabled rules.")
	} else {
		lines = append(lines, strings.Join(blocks, "\n\n"))
	}
	return strings.Join(lines, "\n"), nil
}

func (h *Handlers) syncManagedRulesScript(ctx context.Context, account models.MailAccount, mailboxes []mail.Mailbox) error {
	items, err := h.svc.Store().ListMailRules(ctx, account.ID)
	if err != nil {
		return err
	}
	body, err := compileManagedMailRules(items, mailboxes)
	if err != nil {
		return err
	}
	if err := validateSieveScript(body); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(body))
	if _, err := h.svc.Store().UpsertSieveScript(ctx, models.SieveScript{
		ID:          uuid.NewString(),
		AccountID:   account.ID,
		ScriptName:  managedRulesScriptName,
		ScriptBody:  body,
		ChecksumSHA: hex.EncodeToString(sum[:]),
		Source:      "managed",
	}); err != nil {
		return err
	}
	activeScript, err := h.svc.Store().ActiveSieveScriptName(ctx, account.ID)
	if err != nil {
		return err
	}
	if activeScript == "" || activeScript == managedRulesScriptName {
		if err := h.svc.Store().ActivateSieveScript(ctx, account.ID, managedRulesScriptName); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handlers) writeMailRulesResponse(w http.ResponseWriter, r *http.Request, account models.MailAccount, mailboxes []mail.Mailbox) {
	items, err := h.svc.Store().ListMailRules(r.Context(), account.ID)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "rules_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	activeScript, err := h.svc.Store().ActiveSieveScriptName(r.Context(), account.ID)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "rules_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	activeScript = strings.TrimSpace(activeScript)
	junkMailbox := ruleMailboxByRole(mailboxes, "junk")
	util.WriteJSON(w, http.StatusOK, map[string]any{
		"items":                 items,
		"managed_script_name":   managedRulesScriptName,
		"active_script_name":    activeScript,
		"managed_script_active": activeScript == managedRulesScriptName,
		"custom_script_active":  activeScript != "" && activeScript != managedRulesScriptName,
		"junk_mailbox_name":     junkMailbox,
		"mailboxes":             mailboxes,
	})
}

func (h *Handlers) V2ListAccountRules(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	mailboxes, _, account, _, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.writeMailRulesResponse(w, r, account, mailboxes)
}

func (h *Handlers) V2CreateAccountRule(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	mailboxes, _, account, _, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req mailRuleMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if err := validateMailRule(req); err != nil {
		util.WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	existing, err := h.svc.Store().ListMailRules(r.Context(), account.ID)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "rules_save_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	rule := models.MailRule{
		AccountID:  account.ID,
		Name:       strings.TrimSpace(req.Name),
		Enabled:    req.Enabled == nil || *req.Enabled,
		MatchMode:  req.MatchMode,
		Conditions: trimMailRuleConditions(req.Conditions),
		Actions:    trimMailRuleActions(req.Actions),
	}
	created, err := h.svc.Store().CreateMailRule(r.Context(), rule)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "rules_save_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if req.Position != nil {
		position := *req.Position
		if position < 0 {
			position = 0
		}
		order := make([]string, 0, len(existing)+1)
		inserted := false
		for idx, item := range existing {
			if !inserted && idx >= position {
				order = append(order, created.ID)
				inserted = true
			}
			order = append(order, item.ID)
		}
		if !inserted {
			order = append(order, created.ID)
		}
		if err := h.svc.Store().ReorderMailRules(r.Context(), account.ID, order); err != nil {
			util.WriteError(w, http.StatusInternalServerError, "rules_save_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
	}
	if err := h.syncManagedRulesScript(r.Context(), account, mailboxes); err != nil {
		var required specialMailboxRequiredError
		if errors.As(err, &required) {
			writeSpecialMailboxRequired(w, r, required.Role)
			return
		}
		util.WriteError(w, http.StatusBadRequest, "rules_compile_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.writeMailRulesResponse(w, r, account, mailboxes)
}

func (h *Handlers) V2UpdateAccountRule(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	mailboxes, _, account, _, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	current, err := h.svc.Store().GetMailRuleByID(r.Context(), account.ID, chi.URLParam(r, "rule_id"))
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "rule_not_found", "rule not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "rules_save_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req mailRuleMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		req.Name = current.Name
	}
	if req.Enabled == nil {
		req.Enabled = &current.Enabled
	}
	if strings.TrimSpace(req.MatchMode) == "" {
		req.MatchMode = current.MatchMode
	}
	if req.Conditions == (models.MailRuleConditions{}) {
		req.Conditions = current.Conditions
	}
	if req.Actions == (models.MailRuleActions{}) {
		req.Actions = current.Actions
	}
	if err := validateMailRule(req); err != nil {
		util.WriteError(w, http.StatusBadRequest, "bad_request", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	current.Name = strings.TrimSpace(req.Name)
	current.Enabled = *req.Enabled
	current.MatchMode = req.MatchMode
	current.Conditions = trimMailRuleConditions(req.Conditions)
	current.Actions = trimMailRuleActions(req.Actions)
	if req.Position != nil && *req.Position >= 0 {
		current.Position = *req.Position
	}
	if _, err := h.svc.Store().UpdateMailRule(r.Context(), current); err != nil {
		util.WriteError(w, http.StatusInternalServerError, "rules_save_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.syncManagedRulesScript(r.Context(), account, mailboxes); err != nil {
		var required specialMailboxRequiredError
		if errors.As(err, &required) {
			writeSpecialMailboxRequired(w, r, required.Role)
			return
		}
		util.WriteError(w, http.StatusBadRequest, "rules_compile_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.writeMailRulesResponse(w, r, account, mailboxes)
}

func (h *Handlers) V2DeleteAccountRule(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	mailboxes, _, account, _, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().DeleteMailRule(r.Context(), account.ID, chi.URLParam(r, "rule_id")); err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "rule_not_found", "rule not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "rules_delete_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.syncManagedRulesScript(r.Context(), account, mailboxes); err != nil {
		var required specialMailboxRequiredError
		if errors.As(err, &required) {
			writeSpecialMailboxRequired(w, r, required.Role)
			return
		}
		util.WriteError(w, http.StatusBadRequest, "rules_compile_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.writeMailRulesResponse(w, r, account, mailboxes)
}

func (h *Handlers) V2ReorderAccountRules(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	mailboxes, _, account, _, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	var req mailRuleReorderRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	if err := h.svc.Store().ReorderMailRules(r.Context(), account.ID, req.IDs); err != nil {
		util.WriteError(w, http.StatusInternalServerError, "rules_save_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.syncManagedRulesScript(r.Context(), account, mailboxes); err != nil {
		var required specialMailboxRequiredError
		if errors.As(err, &required) {
			writeSpecialMailboxRequired(w, r, required.Role)
			return
		}
		util.WriteError(w, http.StatusBadRequest, "rules_compile_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.writeMailRulesResponse(w, r, account, mailboxes)
}

func (h *Handlers) V2ActivateManagedRuleScript(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	mailboxes, _, account, _, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if err == store.ErrNotFound {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.syncManagedRulesScript(r.Context(), account, mailboxes); err != nil {
		var required specialMailboxRequiredError
		if errors.As(err, &required) {
			writeSpecialMailboxRequired(w, r, required.Role)
			return
		}
		util.WriteError(w, http.StatusBadRequest, "rules_compile_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().ActivateSieveScript(r.Context(), account.ID, managedRulesScriptName); err != nil {
		util.WriteError(w, http.StatusInternalServerError, "rule_activate_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.writeMailRulesResponse(w, r, account, mailboxes)
}

func mailRulesSortedByPosition(items []models.MailRule) []models.MailRule {
	out := append([]models.MailRule(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}
