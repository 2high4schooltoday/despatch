package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"despatch/internal/mail"
	"despatch/internal/middleware"
	"despatch/internal/models"
	"despatch/internal/service"
	"despatch/internal/store"
	"despatch/internal/util"
)

type specialMailboxMappingDTO struct {
	Role        string `json:"role"`
	MailboxName string `json:"mailbox_name"`
}

var specialMailboxRoles = []string{"sent", "archive", "trash", "junk"}

const appDraftsMailboxName = "__despatch_app_drafts__"

const mailboxMutationMovePageSize = 100

var reservedMailboxNames = map[string]struct{}{
	"inbox":              {},
	"drafts":             {},
	"sent":               {},
	"sent messages":      {},
	"trash":              {},
	"deleted messages":   {},
	"archive":            {},
	"all mail":           {},
	"junk":               {},
	"spam":               {},
	appDraftsMailboxName: {},
}

type mailboxMutationRequest struct {
	MailboxName    string `json:"mailbox_name"`
	NewMailboxName string `json:"new_mailbox_name"`
}

type specialMailboxRequiredError struct {
	Role string
}

func (e specialMailboxRequiredError) Error() string {
	label := aggregateMailboxDisplayName(e.Role)
	if label == "" {
		label = strings.TrimSpace(e.Role)
	}
	if label == "" {
		label = "Required"
	}
	return fmt.Sprintf("%s mailbox must be configured before continuing", label)
}

type mailboxDeleteResult struct {
	MovedCount   int
	TrashMailbox string
	ThreadIDs    []string
}

type adminMailboxTarget struct {
	User    models.User
	Login   string
	Pass    string
	Account *models.MailAccount
	Client  mail.Client
}

var errAdminMailboxCredentialsUnavailable = errors.New("admin mailbox credentials unavailable")

func normalizeAggregateMailboxRole(rawRole, mailboxName string) string {
	role := strings.ToLower(strings.TrimSpace(rawRole))
	if role != "" {
		return role
	}
	key := strings.ToLower(strings.TrimSpace(mailboxName))
	switch {
	case key == "inbox" || strings.HasSuffix(key, "/inbox"):
		return "inbox"
	case key == "drafts" || strings.Contains(key, "draft"):
		return "drafts"
	case key == "sent" || key == "sent messages" || strings.Contains(key, "sent"):
		return "sent"
	case key == "trash" || key == "deleted messages" || strings.Contains(key, "trash") || strings.Contains(key, "deleted"):
		return "trash"
	case key == "archive" || strings.Contains(key, "archive") || strings.Contains(key, "all mail"):
		return "archive"
	case key == "junk" || key == "spam" || strings.Contains(key, "junk") || strings.Contains(key, "spam"):
		return "junk"
	default:
		return ""
	}
}

func aggregateMailboxDisplayName(role string) string {
	switch normalizeAggregateMailboxRole(role, "") {
	case "inbox":
		return "Inbox"
	case "drafts":
		return "Drafts"
	case "sent":
		return "Sent"
	case "trash":
		return "Trash"
	case "archive":
		return "Archive"
	case "junk":
		return "Junk"
	default:
		return ""
	}
}

func aggregateMailboxKey(item mail.Mailbox) string {
	role := normalizeAggregateMailboxRole(item.Role, item.Name)
	if role != "" && role != "drafts" {
		return "role:" + role
	}
	return "name:" + strings.ToLower(strings.TrimSpace(item.Name))
}

func mergeAggregateMailboxes(items [][]mail.Mailbox) []mail.Mailbox {
	merged := map[string]mail.Mailbox{}
	order := make([]string, 0, 16)
	for _, group := range items {
		for _, mailbox := range group {
			role := normalizeAggregateMailboxRole(mailbox.Role, mailbox.Name)
			if role == "drafts" {
				continue
			}
			key := aggregateMailboxKey(mailbox)
			current, ok := merged[key]
			if !ok {
				name := strings.TrimSpace(mailbox.Name)
				if role != "" {
					name = aggregateMailboxDisplayName(role)
				}
				current = mail.Mailbox{
					Name:      name,
					Role:      role,
					CanRename: mailbox.CanRename,
					CanDelete: mailbox.CanDelete,
				}
				order = append(order, key)
			}
			current.Unread += mailbox.Unread
			current.Messages += mailbox.Messages
			current.CanRename = current.CanRename || mailbox.CanRename
			current.CanDelete = current.CanDelete || mailbox.CanDelete
			merged[key] = current
		}
	}
	out := make([]mail.Mailbox, 0, len(order))
	for _, key := range order {
		out = append(out, merged[key])
	}
	rank := func(item mail.Mailbox) int {
		switch normalizeAggregateMailboxRole(item.Role, item.Name) {
		case "inbox":
			return 0
		case "sent":
			return 1
		case "trash":
			return 2
		case "archive":
			return 3
		case "junk":
			return 4
		default:
			return 999
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri := rank(out[i])
		rj := rank(out[j])
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(strings.TrimSpace(out[i].Name)) < strings.ToLower(strings.TrimSpace(out[j].Name))
	})
	return out
}

func resolveAggregateMailboxSelection(mailboxes []mail.Mailbox, mailboxName string) []string {
	target := strings.TrimSpace(mailboxName)
	if target == "" {
		return nil
	}
	role := normalizeAggregateMailboxRole("", target)
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	for _, mailbox := range mailboxes {
		name := strings.TrimSpace(mailbox.Name)
		if name == "" {
			continue
		}
		if role != "" && role != "drafts" {
			if normalizeAggregateMailboxRole(mailbox.Role, mailbox.Name) != role {
				continue
			}
		} else if !strings.EqualFold(name, target) {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	return out
}

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

func isReservedMailboxName(name string) bool {
	_, ok := reservedMailboxNames[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func mailboxIsProtected(item mail.Mailbox) bool {
	if strings.EqualFold(strings.TrimSpace(item.Name), appDraftsMailboxName) {
		return true
	}
	return strings.TrimSpace(item.Role) != ""
}

func applyMailboxCapabilities(items []mail.Mailbox) []mail.Mailbox {
	out := make([]mail.Mailbox, len(items))
	copy(out, items)
	for i := range out {
		protected := mailboxIsProtected(out[i])
		out[i].CanRename = !protected
		out[i].CanDelete = !protected
	}
	return out
}

func mailboxByName(items []mail.Mailbox, name string) (mail.Mailbox, bool) {
	target := strings.TrimSpace(name)
	if target == "" {
		return mail.Mailbox{}, false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Name), target) {
			return item, true
		}
	}
	return mail.Mailbox{}, false
}

func writeMailboxMutationIMAPError(w http.ResponseWriter, r *http.Request, err error) {
	if status, code, ok := classifyMailboxMutationIMAPError(err); ok {
		util.WriteError(w, status, code, err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteError(w, http.StatusBadGateway, "imap_error", err.Error(), middleware.RequestID(r.Context()))
}

func writeSpecialMailboxRequired(w http.ResponseWriter, r *http.Request, role string) {
	err := specialMailboxRequiredError{Role: strings.ToLower(strings.TrimSpace(role))}
	util.WriteError(w, http.StatusConflict, "special_mailbox_required", err.Error(), middleware.RequestID(r.Context()))
}

func writeAdminMailboxAccessError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		util.WriteError(w, http.StatusNotFound, "user_not_found", "user not found", middleware.RequestID(r.Context()))
	case errors.Is(err, errAdminMailboxCredentialsUnavailable):
		util.WriteError(w, http.StatusConflict, "mailbox_credentials_unavailable", "stored mailbox credentials are unavailable for this user; reset the password or let the user sign in once, then retry", middleware.RequestID(r.Context()))
	default:
		util.WriteError(w, http.StatusInternalServerError, "admin_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
	}
}

func (h *Handlers) moveMailboxMessagesToTrash(
	ctx context.Context,
	cli mail.Client,
	login, pass, sourceMailbox string,
	resolveTrash func() (string, error),
	afterMove func(mail.MessageSummary, string) error,
) (mailboxDeleteResult, error) {
	result := mailboxDeleteResult{}
	for {
		items, err := cli.ListMessages(ctx, login, pass, sourceMailbox, 1, mailboxMutationMovePageSize)
		if err != nil {
			return result, err
		}
		if len(items) == 0 {
			return result, nil
		}
		if strings.TrimSpace(result.TrashMailbox) == "" {
			trashMailbox, err := resolveTrash()
			if err != nil {
				return result, err
			}
			if strings.TrimSpace(trashMailbox) == "" {
				return result, specialMailboxRequiredError{Role: "trash"}
			}
			result.TrashMailbox = strings.TrimSpace(trashMailbox)
		}
		movedThisPage := 0
		for _, item := range items {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			if err := cli.Move(ctx, login, pass, id, result.TrashMailbox); err != nil {
				return result, err
			}
			movedThisPage++
			result.MovedCount++
			if afterMove != nil {
				if err := afterMove(item, result.TrashMailbox); err != nil {
					return result, err
				}
			}
		}
		if movedThisPage == 0 {
			return result, fmt.Errorf("mailbox contains messages without ids")
		}
	}
}

func (h *Handlers) listMailboxesWithSpecialRoles(r *http.Request) ([]mail.Mailbox, map[string]string, string, string, error) {
	u, _ := middleware.User(r.Context())
	pass, err := h.sessionMailPassword(r)
	if err != nil {
		return nil, nil, "", "", err
	}
	mailLogin := service.MailAuthLogin(u)
	items, err := h.rawMailboxes(r.Context(), mailLogin, pass)
	if err != nil {
		return nil, nil, mailLogin, pass, err
	}
	mappings, err := h.svc.Store().ListSpecialMailboxMappings(r.Context(), u.ID, mailLogin)
	if err != nil {
		return nil, nil, mailLogin, pass, err
	}
	return applyMailboxCapabilities(applySpecialMailboxRoles(items, mappings)), mappings, mailLogin, pass, nil
}

func (h *Handlers) findPrimaryUserMailAccount(ctx context.Context, u models.User) (*models.MailAccount, error) {
	accounts, err := h.svc.Store().ListMailAccounts(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	targetLogin := strings.TrimSpace(service.MailAuthLogin(u))
	for i := range accounts {
		if targetLogin != "" && strings.EqualFold(strings.TrimSpace(accounts[i].Login), targetLogin) {
			account := accounts[i]
			return &account, nil
		}
	}
	if len(accounts) == 1 {
		account := accounts[0]
		return &account, nil
	}
	return nil, nil
}

func (h *Handlers) resolveAdminMailboxTarget(ctx context.Context, userID string) (adminMailboxTarget, error) {
	u, err := h.svc.Store().GetUserByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return adminMailboxTarget{}, err
	}
	account, err := h.findPrimaryUserMailAccount(ctx, u)
	if err != nil {
		return adminMailboxTarget{}, err
	}
	if secretEnc, ok, err := h.svc.Store().GetUserMailSecret(ctx, u.ID); err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return adminMailboxTarget{}, err
		}
	} else if ok && strings.TrimSpace(secretEnc) != "" {
		pass, decErr := util.DecryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), secretEnc)
		if decErr == nil && strings.TrimSpace(pass) != "" {
			target := adminMailboxTarget{
				User:   u,
				Login:  strings.TrimSpace(service.MailAuthLogin(u)),
				Pass:   pass,
				Client: h.svc.Mail(),
			}
			if account != nil {
				target.Account = account
				target.Client = h.accountMailClient(*account)
				if login := strings.TrimSpace(account.Login); login != "" {
					target.Login = login
				}
			}
			if strings.TrimSpace(target.Login) != "" {
				return target, nil
			}
		}
	}
	if account != nil && strings.TrimSpace(account.SecretEnc) != "" {
		pass, err := h.accountMailSecret(*account)
		if err != nil {
			return adminMailboxTarget{}, err
		}
		if strings.TrimSpace(pass) != "" {
			return adminMailboxTarget{
				User:    u,
				Login:   strings.TrimSpace(account.Login),
				Pass:    pass,
				Account: account,
				Client:  h.accountMailClient(*account),
			}, nil
		}
	}
	return adminMailboxTarget{}, errAdminMailboxCredentialsUnavailable
}

func (h *Handlers) listAdminUserMailboxes(ctx context.Context, userID string) ([]mail.Mailbox, adminMailboxTarget, error) {
	target, err := h.resolveAdminMailboxTarget(ctx, userID)
	if err != nil {
		return nil, adminMailboxTarget{}, err
	}
	if target.Account != nil {
		items, err := h.rawAccountMailboxes(ctx, *target.Account, target.Pass)
		if err != nil {
			return nil, adminMailboxTarget{}, err
		}
		counts, err := h.svc.Store().ListIndexedMailboxCounts(ctx, target.Account.ID)
		if err != nil {
			return nil, adminMailboxTarget{}, err
		}
		mappings, err := h.svc.Store().ListMailboxMappings(ctx, target.Account.ID)
		if err != nil {
			return nil, adminMailboxTarget{}, err
		}
		items = mergeMailboxCounts(items, counts)
		return applyMailboxCapabilities(applySpecialMailboxRoles(items, mailboxMappingsOverlay(mappings))), target, nil
	}
	items, err := h.rawMailboxes(ctx, target.Login, target.Pass)
	if err != nil {
		return nil, adminMailboxTarget{}, err
	}
	mappings, err := h.svc.Store().ListSpecialMailboxMappings(ctx, target.User.ID, target.Login)
	if err != nil {
		return nil, adminMailboxTarget{}, err
	}
	return applyMailboxCapabilities(applySpecialMailboxRoles(items, mappings)), target, nil
}

func (h *Handlers) invalidateAdminMailboxTargetCaches(target adminMailboxTarget) {
	h.invalidateMailCaches(target.Login)
	if target.Account != nil {
		h.mailboxCache.invalidate(accountMailboxCacheKey(*target.Account))
	}
}

func (h *Handlers) resolveAdminUserSpecialMailboxByRole(ctx context.Context, target adminMailboxTarget, role string) (string, error) {
	if target.Account != nil {
		items, err := h.rawAccountMailboxes(ctx, *target.Account, target.Pass)
		if err != nil {
			return "", err
		}
		mappings, err := h.svc.Store().ListMailboxMappings(ctx, target.Account.ID)
		if err != nil {
			return "", err
		}
		return resolveSpecialMailboxFromAvailable(items, mailboxMappingsOverlay(mappings), role), nil
	}
	items, err := h.rawMailboxes(ctx, target.Login, target.Pass)
	if err != nil {
		return "", err
	}
	mappings, err := h.svc.Store().ListSpecialMailboxMappings(ctx, target.User.ID, target.Login)
	if err != nil {
		return "", err
	}
	return resolveSpecialMailboxFromAvailable(items, mappings, role), nil
}

func (h *Handlers) resolveSessionSpecialMailboxByRole(ctx context.Context, u models.User, pass, role string) (string, error) {
	mailLogin := service.MailAuthLogin(u)
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

func mailboxMappingsOverlay(items []models.MailboxMapping) map[string]string {
	out := map[string]string{}
	for _, role := range specialMailboxRoles {
		if mapped := resolveMappedMailboxByRole(items, role); mapped != "" {
			out[role] = mapped
		}
	}
	return out
}

func accountMailboxCacheKey(account models.MailAccount) string {
	return "account:" + strings.TrimSpace(account.ID) + "\x00" + strings.TrimSpace(account.Login)
}

func mergeMailboxCounts(mailboxes []mail.Mailbox, counts []mail.Mailbox) []mail.Mailbox {
	out := make([]mail.Mailbox, len(mailboxes))
	copy(out, mailboxes)
	byName := map[string]mail.Mailbox{}
	for _, item := range counts {
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item
	}
	for i := range out {
		key := strings.ToLower(strings.TrimSpace(out[i].Name))
		if key == "" {
			continue
		}
		if count, ok := byName[key]; ok {
			out[i].Unread = count.Unread
			out[i].Messages = count.Messages
		}
	}
	return out
}

func (h *Handlers) accountMailClient(account models.MailAccount) mail.Client {
	cfg := h.cfg
	cfg.IMAPHost = account.IMAPHost
	cfg.IMAPPort = account.IMAPPort
	cfg.IMAPTLS = account.IMAPTLS
	cfg.IMAPStartTLS = account.IMAPStartTLS
	cfg.SMTPHost = account.SMTPHost
	cfg.SMTPPort = account.SMTPPort
	cfg.SMTPTLS = account.SMTPTLS
	cfg.SMTPStartTLS = account.SMTPStartTLS
	return mailClientFactory(cfg)
}

func (h *Handlers) accountMailSecret(account models.MailAccount) (string, error) {
	return util.DecryptString(util.Derive32ByteKey(h.cfg.SessionEncryptKey), account.SecretEnc)
}

func (h *Handlers) rawAccountMailboxes(ctx context.Context, account models.MailAccount, pass string) ([]mail.Mailbox, error) {
	cacheKey := accountMailboxCacheKey(account)
	cli := h.accountMailClient(account)
	return h.mailboxCache.get(ctx, cacheKey, func(ctx context.Context) ([]mail.Mailbox, error) {
		return cli.ListMailboxes(ctx, account.Login, pass)
	})
}

func (h *Handlers) listAccountMailboxesWithRolesForAccount(ctx context.Context, account models.MailAccount) ([]mail.Mailbox, []models.MailboxMapping, string, error) {
	pass, err := h.accountMailSecret(account)
	if err != nil {
		return nil, nil, "", err
	}
	items, err := h.rawAccountMailboxes(ctx, account, pass)
	if err != nil {
		return nil, nil, pass, err
	}
	counts, err := h.svc.Store().ListIndexedMailboxCounts(ctx, account.ID)
	if err != nil {
		return nil, nil, pass, err
	}
	mappings, err := h.svc.Store().ListMailboxMappings(ctx, account.ID)
	if err != nil {
		return nil, nil, pass, err
	}
	items = mergeMailboxCounts(items, counts)
	return applyMailboxCapabilities(applySpecialMailboxRoles(items, mailboxMappingsOverlay(mappings))), mappings, pass, nil
}

func (h *Handlers) listAccountMailboxesWithRolesBestEffort(ctx context.Context, account models.MailAccount) ([]mail.Mailbox, error) {
	items, _, _, err := h.listAccountMailboxesWithRolesForAccount(ctx, account)
	if err == nil {
		return items, nil
	}
	counts, countErr := h.svc.Store().ListIndexedMailboxCounts(ctx, account.ID)
	if countErr != nil {
		return nil, err
	}
	mappings, mappingsErr := h.svc.Store().ListMailboxMappings(ctx, account.ID)
	if mappingsErr != nil {
		return nil, mappingsErr
	}
	return applyMailboxCapabilities(applySpecialMailboxRoles(counts, mailboxMappingsOverlay(mappings))), nil
}

func (h *Handlers) listAccountMailboxesWithRoles(ctx context.Context, u models.User, accountID string) ([]mail.Mailbox, []models.MailboxMapping, models.MailAccount, string, error) {
	account, err := h.svc.Store().GetMailAccountByID(ctx, u.ID, strings.TrimSpace(accountID))
	if err != nil {
		return nil, nil, models.MailAccount{}, "", err
	}
	items, mappings, pass, err := h.listAccountMailboxesWithRolesForAccount(ctx, account)
	if err != nil {
		return nil, nil, account, pass, err
	}
	return items, mappings, account, pass, nil
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

func (h *Handlers) AdminListUserMailboxes(w http.ResponseWriter, r *http.Request) {
	items, _, err := h.listAdminUserMailboxes(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeAdminMailboxAccessError(w, r, err)
		return
	}
	util.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) AdminCreateUserMailbox(w http.ResponseWriter, r *http.Request) {
	var req mailboxMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	targetName := strings.TrimSpace(req.MailboxName)
	if targetName == "" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}
	if isReservedMailboxName(targetName) {
		util.WriteError(w, http.StatusBadRequest, "mailbox_protected", "mailbox is protected", middleware.RequestID(r.Context()))
		return
	}
	items, target, err := h.listAdminUserMailboxes(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeAdminMailboxAccessError(w, r, err)
		return
	}
	if _, found := mailboxByName(items, targetName); found {
		util.WriteError(w, http.StatusConflict, "mailbox_exists", "mailbox already exists", middleware.RequestID(r.Context()))
		return
	}
	if err := target.Client.CreateMailbox(r.Context(), target.Login, target.Pass, targetName); err != nil {
		writeMailboxMutationIMAPError(w, r, err)
		return
	}
	h.invalidateAdminMailboxTargetCaches(target)
	items, _, err = h.listAdminUserMailboxes(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeAdminMailboxAccessError(w, r, err)
		return
	}
	actualName := targetName
	if current, found := mailboxByName(items, targetName); found {
		actualName = strings.TrimSpace(current.Name)
	}
	util.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":       "ok",
		"mailbox_name": actualName,
		"mailboxes":    items,
	})
}

func (h *Handlers) AdminDeleteUserMailbox(w http.ResponseWriter, r *http.Request) {
	var req mailboxMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	targetName := strings.TrimSpace(req.MailboxName)
	if targetName == "" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}
	items, target, err := h.listAdminUserMailboxes(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeAdminMailboxAccessError(w, r, err)
		return
	}
	current, found := mailboxByName(items, targetName)
	if !found {
		util.WriteError(w, http.StatusNotFound, "mailbox_not_found", "mailbox does not exist", middleware.RequestID(r.Context()))
		return
	}
	if !current.CanDelete {
		util.WriteError(w, http.StatusBadRequest, "mailbox_protected", "mailbox is protected", middleware.RequestID(r.Context()))
		return
	}

	movedThreadIDs := make([]string, 0, 8)
	var afterMove func(mail.MessageSummary, string) error
	if target.Account != nil {
		afterMove = func(item mail.MessageSummary, trashMailbox string) error {
			msg, msgErr := h.svc.Store().GetIndexedMessageByID(r.Context(), target.Account.ID, item.ID)
			if msgErr == nil && strings.TrimSpace(msg.ThreadID) != "" {
				movedThreadIDs = append(movedThreadIDs, msg.ThreadID)
			} else if msgErr != nil && !errors.Is(msgErr, store.ErrNotFound) {
				return msgErr
			}
			if moveErr := h.svc.Store().MoveIndexedMessageMailbox(r.Context(), target.Account.ID, item.ID, trashMailbox); moveErr != nil && !errors.Is(moveErr, store.ErrNotFound) {
				return moveErr
			}
			return nil
		}
	}

	result, err := h.moveMailboxMessagesToTrash(
		r.Context(),
		target.Client,
		target.Login,
		target.Pass,
		current.Name,
		func() (string, error) {
			return h.resolveAdminUserSpecialMailboxByRole(r.Context(), target, "trash")
		},
		afterMove,
	)
	if err != nil {
		var required specialMailboxRequiredError
		if errors.As(err, &required) {
			writeSpecialMailboxRequired(w, r, required.Role)
			return
		}
		writeMailboxMutationIMAPError(w, r, err)
		return
	}
	if err := target.Client.DeleteMailbox(r.Context(), target.Login, target.Pass, current.Name); err != nil {
		writeMailboxMutationIMAPError(w, r, err)
		return
	}
	if target.Account != nil {
		if err := h.svc.Store().DeleteMailboxMappingsByMailbox(r.Context(), target.Account.ID, current.Name); err != nil {
			util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		threadIDs, err := h.svc.Store().DeleteIndexedMailbox(r.Context(), target.Account.ID, current.Name)
		if err != nil {
			util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		threadIDs = append(threadIDs, movedThreadIDs...)
		if err := h.svc.Store().DeleteSyncState(r.Context(), target.Account.ID, current.Name); err != nil {
			util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		if err := h.svc.Store().RefreshThreadIndex(r.Context(), target.Account.ID, threadIDs); err != nil {
			util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
	} else {
		if err := h.svc.Store().DeleteSpecialMailboxMappingsByMailbox(r.Context(), target.User.ID, target.Login, current.Name); err != nil {
			util.WriteError(w, http.StatusInternalServerError, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
	}
	h.invalidateAdminMailboxTargetCaches(target)

	items, _, err = h.listAdminUserMailboxes(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeAdminMailboxAccessError(w, r, err)
		return
	}
	payload := map[string]any{
		"status":       "ok",
		"mailbox_name": strings.TrimSpace(current.Name),
		"mailboxes":    items,
	}
	if result.MovedCount > 0 {
		payload["moved_count"] = result.MovedCount
		payload["trash_mailbox"] = result.TrashMailbox
	}
	util.WriteJSON(w, http.StatusOK, payload)
}

func (h *Handlers) CreateMailbox(w http.ResponseWriter, r *http.Request) {
	var req mailboxMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	target := strings.TrimSpace(req.MailboxName)
	if target == "" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}
	if isReservedMailboxName(target) {
		util.WriteError(w, http.StatusBadRequest, "mailbox_protected", "mailbox is protected", middleware.RequestID(r.Context()))
		return
	}

	items, _, mailLogin, pass, err := h.listMailboxesWithSpecialRoles(r)
	if err != nil {
		if isSessionMailAuthError(err) {
			h.writeMailAuthError(w, r, err)
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if _, found := mailboxByName(items, target); found {
		util.WriteError(w, http.StatusConflict, "mailbox_exists", "mailbox already exists", middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Mail().CreateMailbox(r.Context(), mailLogin, pass, target); err != nil {
		writeMailboxMutationIMAPError(w, r, err)
		return
	}
	h.invalidateMailCaches(mailLogin)

	items, _, _, _, err = h.listMailboxesWithSpecialRoles(r)
	if err != nil {
		if isSessionMailAuthError(err) {
			h.writeMailAuthError(w, r, err)
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	actualName := target
	if current, found := mailboxByName(items, target); found {
		actualName = strings.TrimSpace(current.Name)
	}
	util.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":       "ok",
		"mailbox_name": actualName,
		"mailboxes":    items,
	})
}

func (h *Handlers) RenameMailbox(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req mailboxMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	sourceName := strings.TrimSpace(req.MailboxName)
	targetName := strings.TrimSpace(req.NewMailboxName)
	if sourceName == "" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}
	if targetName == "" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "new_mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}
	if isReservedMailboxName(targetName) {
		util.WriteError(w, http.StatusBadRequest, "mailbox_protected", "mailbox is protected", middleware.RequestID(r.Context()))
		return
	}

	items, _, mailLogin, pass, err := h.listMailboxesWithSpecialRoles(r)
	if err != nil {
		if isSessionMailAuthError(err) {
			h.writeMailAuthError(w, r, err)
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	source, found := mailboxByName(items, sourceName)
	if !found {
		util.WriteError(w, http.StatusNotFound, "mailbox_not_found", "mailbox does not exist", middleware.RequestID(r.Context()))
		return
	}
	if !source.CanRename {
		util.WriteError(w, http.StatusBadRequest, "mailbox_protected", "mailbox is protected", middleware.RequestID(r.Context()))
		return
	}
	if target, exists := mailboxByName(items, targetName); exists && !strings.EqualFold(strings.TrimSpace(target.Name), strings.TrimSpace(source.Name)) {
		util.WriteError(w, http.StatusConflict, "mailbox_exists", "mailbox already exists", middleware.RequestID(r.Context()))
		return
	}
	if !strings.EqualFold(strings.TrimSpace(source.Name), targetName) {
		if err := h.svc.Mail().RenameMailbox(r.Context(), mailLogin, pass, source.Name, targetName); err != nil {
			writeMailboxMutationIMAPError(w, r, err)
			return
		}
		if err := h.svc.Store().RenameSpecialMailboxMappings(r.Context(), u.ID, mailLogin, source.Name, targetName); err != nil {
			util.WriteError(w, http.StatusInternalServerError, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		h.invalidateMailCaches(mailLogin)
	}

	items, _, _, _, err = h.listMailboxesWithSpecialRoles(r)
	if err != nil {
		if isSessionMailAuthError(err) {
			h.writeMailAuthError(w, r, err)
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	actualName := targetName
	if current, found := mailboxByName(items, targetName); found {
		actualName = strings.TrimSpace(current.Name)
	}
	util.WriteJSON(w, http.StatusOK, map[string]any{
		"status":                "ok",
		"mailbox_name":          actualName,
		"previous_mailbox_name": strings.TrimSpace(source.Name),
		"mailboxes":             items,
	})
}

func (h *Handlers) DeleteMailbox(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req mailboxMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	target := strings.TrimSpace(req.MailboxName)
	if target == "" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}

	items, _, mailLogin, pass, err := h.listMailboxesWithSpecialRoles(r)
	if err != nil {
		if isSessionMailAuthError(err) {
			h.writeMailAuthError(w, r, err)
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	current, found := mailboxByName(items, target)
	if !found {
		util.WriteError(w, http.StatusNotFound, "mailbox_not_found", "mailbox does not exist", middleware.RequestID(r.Context()))
		return
	}
	if !current.CanDelete {
		util.WriteError(w, http.StatusBadRequest, "mailbox_protected", "mailbox is protected", middleware.RequestID(r.Context()))
		return
	}
	result, err := h.moveMailboxMessagesToTrash(
		r.Context(),
		h.svc.Mail(),
		mailLogin,
		pass,
		current.Name,
		func() (string, error) {
			return h.resolveSessionSpecialMailboxByRole(r.Context(), u, pass, "trash")
		},
		nil,
	)
	if err != nil {
		var required specialMailboxRequiredError
		if errors.As(err, &required) {
			writeSpecialMailboxRequired(w, r, required.Role)
			return
		}
		writeMailboxMutationIMAPError(w, r, err)
		return
	}
	if err := h.svc.Mail().DeleteMailbox(r.Context(), mailLogin, pass, current.Name); err != nil {
		writeMailboxMutationIMAPError(w, r, err)
		return
	}
	if err := h.svc.Store().DeleteSpecialMailboxMappingsByMailbox(r.Context(), u.ID, mailLogin, current.Name); err != nil {
		util.WriteError(w, http.StatusInternalServerError, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	h.invalidateMailCaches(mailLogin)

	items, _, _, _, err = h.listMailboxesWithSpecialRoles(r)
	if err != nil {
		if isSessionMailAuthError(err) {
			h.writeMailAuthError(w, r, err)
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "special_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	payload := map[string]any{
		"status":       "ok",
		"mailbox_name": strings.TrimSpace(current.Name),
		"mailboxes":    items,
	}
	if result.MovedCount > 0 {
		payload["moved_count"] = result.MovedCount
		payload["trash_mailbox"] = result.TrashMailbox
	}
	util.WriteJSON(w, http.StatusOK, payload)
}

func (h *Handlers) UpsertSpecialMailbox(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	role := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "role")))
	if role != "sent" && role != "archive" && role != "trash" && role != "junk" {
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
	items = applyMailboxCapabilities(applySpecialMailboxRoles(items, mappings))
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

func (h *Handlers) V2ListAccountMailboxes(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	items, _, _, _, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, http.StatusOK, items)
}

func (h *Handlers) V2ListAggregateMailboxes(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	accounts, err := h.svc.Store().ListMailAccounts(r.Context(), u.ID)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "accounts_list_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	items := make([][]mail.Mailbox, 0, len(accounts))
	var firstErr error
	for _, account := range accounts {
		mailboxes, mailboxErr := h.listAccountMailboxesWithRolesBestEffort(r.Context(), account)
		if mailboxErr != nil {
			if firstErr == nil {
				firstErr = mailboxErr
			}
			continue
		}
		items = append(items, mailboxes)
	}
	merged := mergeAggregateMailboxes(items)
	if len(accounts) > 0 && len(merged) == 0 && firstErr != nil {
		util.WriteError(w, http.StatusInternalServerError, "aggregate_mailboxes_failed", firstErr.Error(), middleware.RequestID(r.Context()))
		return
	}
	util.WriteJSON(w, http.StatusOK, merged)
}

func (h *Handlers) V2CreateAccountMailbox(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req mailboxMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	target := strings.TrimSpace(req.MailboxName)
	if target == "" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}
	if isReservedMailboxName(target) {
		util.WriteError(w, http.StatusBadRequest, "mailbox_protected", "mailbox is protected", middleware.RequestID(r.Context()))
		return
	}

	items, _, account, pass, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if _, found := mailboxByName(items, target); found {
		util.WriteError(w, http.StatusConflict, "mailbox_exists", "mailbox already exists", middleware.RequestID(r.Context()))
		return
	}
	cli := h.accountMailClient(account)
	if err := cli.CreateMailbox(r.Context(), account.Login, pass, target); err != nil {
		writeMailboxMutationIMAPError(w, r, err)
		return
	}
	h.mailboxCache.invalidate(accountMailboxCacheKey(account))
	h.mailboxCache.invalidate(account.Login)

	items, _, _, _, err = h.listAccountMailboxesWithRoles(r.Context(), u, account.ID)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	actualName := target
	if current, found := mailboxByName(items, target); found {
		actualName = strings.TrimSpace(current.Name)
	}
	util.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":       "ok",
		"mailbox_name": actualName,
		"mailboxes":    items,
	})
}

func (h *Handlers) V2RenameAccountMailbox(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req mailboxMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	sourceName := strings.TrimSpace(req.MailboxName)
	targetName := strings.TrimSpace(req.NewMailboxName)
	if sourceName == "" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}
	if targetName == "" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "new_mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}
	if isReservedMailboxName(targetName) {
		util.WriteError(w, http.StatusBadRequest, "mailbox_protected", "mailbox is protected", middleware.RequestID(r.Context()))
		return
	}

	items, _, account, pass, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	source, found := mailboxByName(items, sourceName)
	if !found {
		util.WriteError(w, http.StatusNotFound, "mailbox_not_found", "mailbox does not exist", middleware.RequestID(r.Context()))
		return
	}
	if !source.CanRename {
		util.WriteError(w, http.StatusBadRequest, "mailbox_protected", "mailbox is protected", middleware.RequestID(r.Context()))
		return
	}
	if target, exists := mailboxByName(items, targetName); exists && !strings.EqualFold(strings.TrimSpace(target.Name), strings.TrimSpace(source.Name)) {
		util.WriteError(w, http.StatusConflict, "mailbox_exists", "mailbox already exists", middleware.RequestID(r.Context()))
		return
	}
	if !strings.EqualFold(strings.TrimSpace(source.Name), targetName) {
		cli := h.accountMailClient(account)
		if err := cli.RenameMailbox(r.Context(), account.Login, pass, source.Name, targetName); err != nil {
			writeMailboxMutationIMAPError(w, r, err)
			return
		}
		h.mailboxCache.invalidate(accountMailboxCacheKey(account))
		h.mailboxCache.invalidate(account.Login)
		if err := h.svc.Store().RenameMailboxMappings(r.Context(), account.ID, source.Name, targetName); err != nil {
			util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		threadIDs, err := h.svc.Store().RenameIndexedMailbox(r.Context(), account.ID, source.Name, targetName)
		if err != nil {
			util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		if err := h.svc.Store().DeleteSyncState(r.Context(), account.ID, source.Name); err != nil {
			util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		if err := h.svc.Store().RefreshThreadIndex(r.Context(), account.ID, threadIDs); err != nil {
			util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
			return
		}
	}

	items, _, _, _, err = h.listAccountMailboxesWithRoles(r.Context(), u, account.ID)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	actualName := targetName
	if current, found := mailboxByName(items, targetName); found {
		actualName = strings.TrimSpace(current.Name)
	}
	util.WriteJSON(w, http.StatusOK, map[string]any{
		"status":                "ok",
		"mailbox_name":          actualName,
		"previous_mailbox_name": strings.TrimSpace(source.Name),
		"mailboxes":             items,
	})
}

func (h *Handlers) V2DeleteAccountMailbox(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	var req mailboxMutationRequest
	if err := decodeJSON(w, r, &req, jsonLimitMutation, false); err != nil {
		writeJSONDecodeError(w, r, err)
		return
	}
	target := strings.TrimSpace(req.MailboxName)
	if target == "" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}

	items, _, account, pass, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	current, found := mailboxByName(items, target)
	if !found {
		util.WriteError(w, http.StatusNotFound, "mailbox_not_found", "mailbox does not exist", middleware.RequestID(r.Context()))
		return
	}
	if !current.CanDelete {
		util.WriteError(w, http.StatusBadRequest, "mailbox_protected", "mailbox is protected", middleware.RequestID(r.Context()))
		return
	}
	cli := h.accountMailClient(account)
	movedThreadIDs := make([]string, 0, 8)
	result, err := h.moveMailboxMessagesToTrash(
		r.Context(),
		cli,
		account.Login,
		pass,
		current.Name,
		func() (string, error) {
			return h.resolveAccountSpecialMailboxByRole(r.Context(), account, pass, "trash", cli)
		},
		func(item mail.MessageSummary, trashMailbox string) error {
			msg, msgErr := h.svc.Store().GetIndexedMessageByID(r.Context(), account.ID, item.ID)
			if msgErr == nil && strings.TrimSpace(msg.ThreadID) != "" {
				movedThreadIDs = append(movedThreadIDs, msg.ThreadID)
			} else if msgErr != nil && !errors.Is(msgErr, store.ErrNotFound) {
				return msgErr
			}
			if moveErr := h.svc.Store().MoveIndexedMessageMailbox(r.Context(), account.ID, item.ID, trashMailbox); moveErr != nil && !errors.Is(moveErr, store.ErrNotFound) {
				return moveErr
			}
			return nil
		},
	)
	if err != nil {
		var required specialMailboxRequiredError
		if errors.As(err, &required) {
			writeSpecialMailboxRequired(w, r, required.Role)
			return
		}
		writeMailboxMutationIMAPError(w, r, err)
		return
	}
	if err := cli.DeleteMailbox(r.Context(), account.Login, pass, current.Name); err != nil {
		writeMailboxMutationIMAPError(w, r, err)
		return
	}
	h.mailboxCache.invalidate(accountMailboxCacheKey(account))
	h.mailboxCache.invalidate(account.Login)
	if err := h.svc.Store().DeleteMailboxMappingsByMailbox(r.Context(), account.ID, current.Name); err != nil {
		util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	threadIDs, err := h.svc.Store().DeleteIndexedMailbox(r.Context(), account.ID, current.Name)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	threadIDs = append(threadIDs, movedThreadIDs...)
	if err := h.svc.Store().DeleteSyncState(r.Context(), account.ID, current.Name); err != nil {
		util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	if err := h.svc.Store().RefreshThreadIndex(r.Context(), account.ID, threadIDs); err != nil {
		util.WriteError(w, http.StatusInternalServerError, "index_update_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}

	items, _, _, _, err = h.listAccountMailboxesWithRoles(r.Context(), u, account.ID)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	payload := map[string]any{
		"status":       "ok",
		"mailbox_name": strings.TrimSpace(current.Name),
		"mailboxes":    items,
	}
	if result.MovedCount > 0 {
		payload["moved_count"] = result.MovedCount
		payload["trash_mailbox"] = result.TrashMailbox
	}
	util.WriteJSON(w, http.StatusOK, payload)
}

func (h *Handlers) V2UpsertAccountSpecialMailbox(w http.ResponseWriter, r *http.Request) {
	u, _ := middleware.User(r.Context())
	role := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "role")))
	if role != "sent" && role != "archive" && role != "trash" && role != "junk" {
		util.WriteError(w, http.StatusBadRequest, "bad_request", "unsupported mailbox role", middleware.RequestID(r.Context()))
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
		util.WriteError(w, http.StatusBadRequest, "bad_request", "mailbox_name is required", middleware.RequestID(r.Context()))
		return
	}
	items, mappings, account, pass, err := h.listAccountMailboxesWithRoles(r.Context(), u, chi.URLParam(r, "id"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			util.WriteError(w, http.StatusNotFound, "account_not_found", "account not found", middleware.RequestID(r.Context()))
			return
		}
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
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
			util.WriteError(w, http.StatusNotFound, "mailbox_not_found", "mailbox does not exist", middleware.RequestID(r.Context()))
			return
		}
		cli := h.accountMailClient(account)
		if err := cli.CreateMailbox(r.Context(), account.Login, pass, target); err != nil {
			util.WriteError(w, http.StatusBadGateway, "imap_error", err.Error(), middleware.RequestID(r.Context()))
			return
		}
		actualName = target
		created = true
	}

	mappingID := ""
	for _, item := range mappings {
		if strings.EqualFold(strings.TrimSpace(item.Role), role) {
			mappingID = strings.TrimSpace(item.ID)
			break
		}
	}
	if mappingID == "" {
		mappingID = uuid.NewString()
	}
	if _, err := h.svc.Store().UpsertMailboxMapping(r.Context(), models.MailboxMapping{
		ID:          mappingID,
		AccountID:   account.ID,
		Role:        role,
		MailboxName: actualName,
		Source:      "manual",
		Priority:    100,
	}); err != nil {
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}

	h.mailboxCache.invalidate(accountMailboxCacheKey(account))
	h.mailboxCache.invalidate(account.Login)

	items, mappings, _, _, err = h.listAccountMailboxesWithRoles(r.Context(), u, account.ID)
	if err != nil {
		util.WriteError(w, http.StatusInternalServerError, "account_mailboxes_failed", err.Error(), middleware.RequestID(r.Context()))
		return
	}
	responseItems := specialMailboxMappingsForResponse(mailboxMappingsOverlay(mappings))
	sort.Slice(responseItems, func(i, j int) bool {
		return responseItems[i].Role < responseItems[j].Role
	})
	util.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"role":         role,
		"mailbox_name": actualName,
		"created":      created,
		"items":        responseItems,
		"mailboxes":    items,
	})
}
