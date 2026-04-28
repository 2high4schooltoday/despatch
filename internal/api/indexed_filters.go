package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"despatch/internal/mail"
	"despatch/internal/models"
)

func normalizeIndexedFilterAccountIDs(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func queryBoolEnabled(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func indexedFilterLocation(ctx context.Context, h *Handlers, userID string) *time.Location {
	prefs, err := h.svc.Store().GetUserPreferences(ctx, userID)
	if err != nil {
		return time.UTC
	}
	name := strings.TrimSpace(prefs.Timezone)
	if name == "" {
		name = strings.TrimSpace(h.svc.DefaultTimezone(ctx))
		if name == "" {
			return time.UTC
		}
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

func parseIndexedCalendarDate(raw string, loc *time.Location, endOfDay bool) (time.Time, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", trimmed, loc)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid date %q", trimmed)
	}
	if endOfDay {
		parsed = parsed.AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	return parsed.UTC(), true, nil
}

func applyLegacyIndexedViewFilter(view string, filter *models.IndexedMessageFilter) {
	if filter == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(view)) {
	case "unread":
		filter.Unread = true
	case "flagged":
		filter.Flagged = true
	case "attachments":
		filter.HasAttachments = true
	case "waiting":
		filter.Waiting = true
	}
	applyTriageIndexedViewFilter(view, filter)
}

func resolveIndexedFilteredAccounts(accounts []models.MailAccount, filterAccountIDs []string) ([]models.MailAccount, error) {
	filterAccountIDs = normalizeIndexedFilterAccountIDs(filterAccountIDs)
	if len(filterAccountIDs) == 0 {
		return accounts, nil
	}
	allowed := make(map[string]models.MailAccount, len(accounts))
	for _, account := range accounts {
		allowed[strings.TrimSpace(account.ID)] = account
	}
	filtered := make([]models.MailAccount, 0, len(filterAccountIDs))
	for _, accountID := range filterAccountIDs {
		account, ok := allowed[accountID]
		if !ok {
			return nil, fmt.Errorf("filter_account_id %q does not belong to current user scope", accountID)
		}
		filtered = append(filtered, account)
	}
	return filtered, nil
}

func (h *Handlers) parseIndexedMessageFilter(ctx context.Context, u models.User, r *http.Request, accounts []models.MailAccount) (models.IndexedMessageFilter, []models.MailAccount, error) {
	filter := models.IndexedMessageFilter{
		Query:          strings.TrimSpace(r.URL.Query().Get("q")),
		From:           strings.TrimSpace(r.URL.Query().Get("from")),
		To:             strings.TrimSpace(r.URL.Query().Get("to")),
		Subject:        strings.TrimSpace(r.URL.Query().Get("subject")),
		Unread:         queryBoolEnabled(r.URL.Query().Get("unread")),
		Flagged:        queryBoolEnabled(r.URL.Query().Get("flagged")),
		HasAttachments: queryBoolEnabled(r.URL.Query().Get("has_attachments")),
		Waiting:        queryBoolEnabled(r.URL.Query().Get("waiting")),
		Snoozed:        queryBoolEnabled(r.URL.Query().Get("snoozed")),
		FollowUp:       queryBoolEnabled(r.URL.Query().Get("follow_up")),
		CategoryID:     strings.TrimSpace(r.URL.Query().Get("category_id")),
		TagIDs:         normalizeIndexedFilterTagIDs(r.URL.Query()["tag_id"]),
		AccountIDs:     normalizeIndexedFilterAccountIDs(r.URL.Query()["filter_account_id"]),
	}
	applyLegacyIndexedViewFilter(r.URL.Query().Get("view"), &filter)
	loc := indexedFilterLocation(ctx, h, u.ID)
	var err error
	filter.DateFrom, filter.HasDateFrom, err = parseIndexedCalendarDate(r.URL.Query().Get("date_from"), loc, false)
	if err != nil {
		return models.IndexedMessageFilter{}, nil, err
	}
	filter.DateTo, filter.HasDateTo, err = parseIndexedCalendarDate(r.URL.Query().Get("date_to"), loc, true)
	if err != nil {
		return models.IndexedMessageFilter{}, nil, err
	}
	if filter.HasDateFrom && filter.HasDateTo && filter.DateTo.Before(filter.DateFrom) {
		return models.IndexedMessageFilter{}, nil, fmt.Errorf("date_to must not be earlier than date_from")
	}
	filteredAccounts, err := resolveIndexedFilteredAccounts(accounts, filter.AccountIDs)
	if err != nil {
		return models.IndexedMessageFilter{}, nil, err
	}
	return filter, filteredAccounts, nil
}

func (h *Handlers) queryIndexedMessages(
	ctx context.Context,
	u models.User,
	accounts []models.MailAccount,
	mailbox string,
	mailboxFilters map[string][]string,
	filter models.IndexedMessageFilter,
	page,
	pageSize int,
	sortOrder string,
	preferSearch bool,
) ([]models.IndexedMessage, int, error) {
	return h.queryIndexedMessagesWithTriage(ctx, u, accounts, mailbox, mailboxFilters, filter, page, pageSize, sortOrder, preferSearch)
}

func presentIndexedMessageSummaries(items []models.IndexedMessage) []mail.MessageSummary {
	out := make([]mail.MessageSummary, 0, len(items))
	for _, item := range items {
		out = append(out, indexedMessageSummary(item))
	}
	return out
}
