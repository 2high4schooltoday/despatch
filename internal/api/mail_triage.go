package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"despatch/internal/mail"
	"despatch/internal/models"
)

const liveTriageScanPageSize = 50
const liveTriageMaxPages = 20

func normalizeIndexedFilterTagIDs(values []string) []string {
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

func applyTriageIndexedViewFilter(view string, filter *models.IndexedMessageFilter) {
	if filter == nil {
		return
	}
	switch strings.ToLower(strings.TrimSpace(view)) {
	case "snoozed":
		filter.Snoozed = true
	case "follow_up", "follow-up":
		filter.FollowUp = true
	}
}

func mailTriageExplicitFilterActive(filter models.IndexedMessageFilter) bool {
	return filter.Snoozed || filter.FollowUp || strings.TrimSpace(filter.CategoryID) != "" || len(filter.TagIDs) > 0
}

func mailConversationFilterActive(filter models.IndexedMessageFilter) bool {
	return filter.Waiting || mailTriageExplicitFilterActive(filter)
}

func modelTriageCategoryToMail(item *models.MailTriageCategoryRef) *mail.TriageCategory {
	if item == nil {
		return nil
	}
	return &mail.TriageCategory{
		ID:   strings.TrimSpace(item.ID),
		Name: strings.TrimSpace(item.Name),
	}
}

func modelTriageTagsToMail(items []models.MailTriageTagRef) []mail.TriageTag {
	out := make([]mail.TriageTag, 0, len(items))
	for _, item := range items {
		out = append(out, mail.TriageTag{
			ID:   strings.TrimSpace(item.ID),
			Name: strings.TrimSpace(item.Name),
		})
	}
	return out
}

func modelTriageStateToMail(item models.MailTriageState) mail.TriageState {
	state := mail.DefaultTriageState()
	if item.SnoozedUntil != nil {
		value := item.SnoozedUntil.UTC()
		state.SnoozedUntil = &value
	}
	if item.ReminderAt != nil {
		value := item.ReminderAt.UTC()
		state.ReminderAt = &value
	}
	state.Category = modelTriageCategoryToMail(item.Category)
	state.Tags = modelTriageTagsToMail(item.Tags)
	state.IsSnoozed = item.IsSnoozed
	state.IsFollowUpDue = item.IsFollowUpDue
	return state
}

func mailTriageStateToModel(item mail.TriageState) models.MailTriageState {
	state := models.DefaultMailTriageState()
	if item.SnoozedUntil != nil {
		value := item.SnoozedUntil.UTC()
		state.SnoozedUntil = &value
	}
	if item.ReminderAt != nil {
		value := item.ReminderAt.UTC()
		state.ReminderAt = &value
	}
	if item.Category != nil {
		state.Category = &models.MailTriageCategoryRef{
			ID:   strings.TrimSpace(item.Category.ID),
			Name: strings.TrimSpace(item.Category.Name),
		}
	}
	for _, tag := range item.Tags {
		state.Tags = append(state.Tags, models.MailTriageTagRef{
			ID:   strings.TrimSpace(tag.ID),
			Name: strings.TrimSpace(tag.Name),
		})
	}
	state.IsSnoozed = item.IsSnoozed
	state.IsFollowUpDue = item.IsFollowUpDue
	return state
}

func liveSummaryTriageTarget(item mail.MessageSummary) models.MailTriageTarget {
	return models.MailTriageTarget{
		Source:    "live",
		AccountID: strings.TrimSpace(item.AccountID),
		ThreadID:  strings.TrimSpace(item.ThreadID),
		Mailbox:   strings.TrimSpace(item.Mailbox),
		Subject:   strings.TrimSpace(item.Subject),
		From:      strings.TrimSpace(item.From),
	}
}

func liveMessageTriageTarget(item mail.Message) models.MailTriageTarget {
	return models.MailTriageTarget{
		Source:    "live",
		AccountID: "",
		ThreadID:  strings.TrimSpace(item.ThreadID),
		Mailbox:   strings.TrimSpace(item.Mailbox),
		Subject:   strings.TrimSpace(item.Subject),
		From:      strings.TrimSpace(item.From),
	}
}

func indexedMessageTriageTarget(item models.IndexedMessage) models.MailTriageTarget {
	return models.MailTriageTarget{
		Source:    "indexed",
		AccountID: strings.TrimSpace(item.AccountID),
		ThreadID:  strings.TrimSpace(item.ThreadID),
		Mailbox:   strings.TrimSpace(item.Mailbox),
		Subject:   strings.TrimSpace(item.Subject),
		From:      strings.TrimSpace(item.FromValue),
	}
}

func decorateLiveMessageSummaryWithTriage(item mail.MessageSummary, triage models.MailThreadTriageState) mail.MessageSummary {
	item.TriageKey = strings.TrimSpace(triage.TriageKey)
	item.Triage = modelTriageStateToMail(triage.Triage)
	return item
}

func decorateLiveMessageWithTriage(item mail.Message, triage models.MailThreadTriageState) mail.Message {
	item.TriageKey = strings.TrimSpace(triage.TriageKey)
	item.Triage = modelTriageStateToMail(triage.Triage)
	return item
}

func decorateIndexedMessageWithTriage(item models.IndexedMessage, triage models.MailThreadTriageState) models.IndexedMessage {
	item.TriageKey = strings.TrimSpace(triage.TriageKey)
	item.Triage = triage.Triage
	return item
}

func decorateLiveMessageSummariesWithTriage(ctx context.Context, h *Handlers, userID string, items []mail.MessageSummary) ([]mail.MessageSummary, error) {
	if len(items) == 0 {
		return []mail.MessageSummary{}, nil
	}
	targets := make([]models.MailTriageTarget, 0, len(items))
	for _, item := range items {
		targets = append(targets, liveSummaryTriageTarget(item))
	}
	states, err := h.svc.Store().GetMailThreadTriageStates(ctx, userID, targets)
	if err != nil {
		return nil, err
	}
	stateByThread := map[string]models.MailThreadTriageState{}
	for _, state := range states {
		key := strings.TrimSpace(state.Target.Source) + "\x00" + strings.TrimSpace(state.Target.AccountID) + "\x00" + strings.TrimSpace(state.Target.ThreadID)
		stateByThread[key] = state
	}
	out := make([]mail.MessageSummary, 0, len(items))
	for _, item := range items {
		key := "live\x00" + strings.TrimSpace(item.AccountID) + "\x00" + strings.TrimSpace(item.ThreadID)
		triage, ok := stateByThread[key]
		if !ok {
			out = append(out, item)
			continue
		}
		out = append(out, decorateLiveMessageSummaryWithTriage(item, triage))
	}
	return out, nil
}

func decorateLiveMessageWithTriageStore(ctx context.Context, h *Handlers, userID string, item mail.Message) (mail.Message, error) {
	states, err := h.svc.Store().GetMailThreadTriageStates(ctx, userID, []models.MailTriageTarget{liveMessageTriageTarget(item)})
	if err != nil {
		return item, err
	}
	if len(states) == 0 {
		return item, nil
	}
	return decorateLiveMessageWithTriage(item, states[0]), nil
}

func decorateIndexedMessagesWithTriage(ctx context.Context, h *Handlers, userID string, items []models.IndexedMessage) ([]models.IndexedMessage, error) {
	if len(items) == 0 {
		return []models.IndexedMessage{}, nil
	}
	targets := make([]models.MailTriageTarget, 0, len(items))
	for _, item := range items {
		targets = append(targets, indexedMessageTriageTarget(item))
	}
	states, err := h.svc.Store().GetMailThreadTriageStates(ctx, userID, targets)
	if err != nil {
		return nil, err
	}
	stateByThread := map[string]models.MailThreadTriageState{}
	for _, state := range states {
		key := strings.TrimSpace(state.Target.Source) + "\x00" + strings.TrimSpace(state.Target.AccountID) + "\x00" + strings.TrimSpace(state.Target.ThreadID)
		stateByThread[key] = state
	}
	out := make([]models.IndexedMessage, 0, len(items))
	for _, item := range items {
		key := "indexed\x00" + strings.TrimSpace(item.AccountID) + "\x00" + strings.TrimSpace(item.ThreadID)
		triage, ok := stateByThread[key]
		if !ok {
			item.Triage = models.DefaultMailTriageState()
			out = append(out, item)
			continue
		}
		out = append(out, decorateIndexedMessageWithTriage(item, triage))
	}
	return out, nil
}

func decorateIndexedMessageWithTriageStore(ctx context.Context, h *Handlers, userID string, item models.IndexedMessage) (models.IndexedMessage, error) {
	items, err := decorateIndexedMessagesWithTriage(ctx, h, userID, []models.IndexedMessage{item})
	if err != nil || len(items) == 0 {
		return item, err
	}
	return items[0], nil
}

func mailTriageTagMatch(item models.MailTriageState, required []string) bool {
	if len(required) == 0 {
		return true
	}
	for _, tag := range item.Tags {
		for _, requiredID := range required {
			if strings.TrimSpace(tag.ID) == strings.TrimSpace(requiredID) {
				return true
			}
		}
	}
	return false
}

func mailTriageMatchesFilter(item models.MailTriageState, filter models.IndexedMessageFilter) bool {
	if filter.Snoozed && !item.IsSnoozed {
		return false
	}
	if filter.FollowUp && item.ReminderAt == nil {
		return false
	}
	if filter.CategoryID != "" {
		if item.Category == nil || strings.TrimSpace(item.Category.ID) != strings.TrimSpace(filter.CategoryID) {
			return false
		}
	}
	if !mailTriageTagMatch(item, filter.TagIDs) {
		return false
	}
	return true
}

func mailTriageShouldHideSnoozed(item models.MailTriageState, filter models.IndexedMessageFilter) bool {
	return item.IsSnoozed && !mailTriageExplicitFilterActive(filter)
}

func collapseIndexedMessagesByThread(items []models.IndexedMessage) []models.IndexedMessage {
	seen := map[string]struct{}{}
	out := make([]models.IndexedMessage, 0, len(items))
	for _, item := range items {
		threadID := strings.TrimSpace(item.ThreadID)
		if threadID == "" {
			threadID = strings.TrimSpace(item.ID)
		}
		if threadID == "" {
			continue
		}
		if _, ok := seen[threadID]; ok {
			continue
		}
		seen[threadID] = struct{}{}
		out = append(out, item)
	}
	return out
}

func collapseLiveMessageSummariesByThread(items []mail.MessageSummary) []mail.MessageSummary {
	seen := map[string]struct{}{}
	out := make([]mail.MessageSummary, 0, len(items))
	for _, item := range items {
		threadID := strings.TrimSpace(item.ThreadID)
		if threadID == "" {
			threadID = strings.TrimSpace(item.ID)
		}
		if threadID == "" {
			continue
		}
		if _, ok := seen[threadID]; ok {
			continue
		}
		seen[threadID] = struct{}{}
		out = append(out, item)
	}
	return out
}

func indexedTriageSortTime(item models.IndexedMessage) time.Time {
	if !item.DateHeader.IsZero() {
		return item.DateHeader
	}
	return item.InternalDate
}

func liveTriageSortTime(item mail.MessageSummary) time.Time {
	return item.Date
}

func triageTimeLess(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.Before(*b)
}

func sortIndexedMessagesForTriageView(items []models.IndexedMessage, filter models.IndexedMessageFilter) {
	sort.SliceStable(items, func(i, j int) bool {
		a := items[i]
		b := items[j]
		if filter.Snoozed {
			if triageTimeLess(a.Triage.SnoozedUntil, b.Triage.SnoozedUntil) {
				return true
			}
			if triageTimeLess(b.Triage.SnoozedUntil, a.Triage.SnoozedUntil) {
				return false
			}
		}
		if filter.FollowUp {
			if a.Triage.IsFollowUpDue != b.Triage.IsFollowUpDue {
				return a.Triage.IsFollowUpDue
			}
			if triageTimeLess(a.Triage.ReminderAt, b.Triage.ReminderAt) {
				return true
			}
			if triageTimeLess(b.Triage.ReminderAt, a.Triage.ReminderAt) {
				return false
			}
		}
		at := indexedTriageSortTime(a)
		bt := indexedTriageSortTime(b)
		if !at.Equal(bt) {
			return at.After(bt)
		}
		return strings.TrimSpace(a.ID) < strings.TrimSpace(b.ID)
	})
}

func sortLiveSummariesForTriageView(items []mail.MessageSummary, filter models.IndexedMessageFilter) {
	sort.SliceStable(items, func(i, j int) bool {
		a := items[i]
		b := items[j]
		if filter.Snoozed {
			if triageTimeLess(a.Triage.SnoozedUntil, b.Triage.SnoozedUntil) {
				return true
			}
			if triageTimeLess(b.Triage.SnoozedUntil, a.Triage.SnoozedUntil) {
				return false
			}
		}
		if filter.FollowUp {
			if a.Triage.IsFollowUpDue != b.Triage.IsFollowUpDue {
				return a.Triage.IsFollowUpDue
			}
			if triageTimeLess(a.Triage.ReminderAt, b.Triage.ReminderAt) {
				return true
			}
			if triageTimeLess(b.Triage.ReminderAt, a.Triage.ReminderAt) {
				return false
			}
		}
		at := liveTriageSortTime(a)
		bt := liveTriageSortTime(b)
		if !at.Equal(bt) {
			return at.After(bt)
		}
		return strings.TrimSpace(a.ID) < strings.TrimSpace(b.ID)
	})
}

func liveSummaryMatchesTriageFilter(item mail.MessageSummary, filter models.IndexedMessageFilter) bool {
	return mailTriageMatchesFilter(mailTriageStateToModel(item.Triage), filter)
}

func liveSummaryShouldHideSnoozed(item mail.MessageSummary, filter models.IndexedMessageFilter) bool {
	return mailTriageShouldHideSnoozed(mailTriageStateToModel(item.Triage), filter)
}

func stripIndexedTriageFilter(filter models.IndexedMessageFilter) models.IndexedMessageFilter {
	filter.Waiting = false
	filter.Snoozed = false
	filter.FollowUp = false
	filter.CategoryID = ""
	filter.TagIDs = nil
	return filter
}

func parseMailTriageOnlyFilter(r *http.Request) models.IndexedMessageFilter {
	filter := models.IndexedMessageFilter{
		Snoozed:    queryBoolEnabled(r.URL.Query().Get("snoozed")),
		FollowUp:   queryBoolEnabled(r.URL.Query().Get("follow_up")),
		CategoryID: strings.TrimSpace(r.URL.Query().Get("category_id")),
		TagIDs:     normalizeIndexedFilterTagIDs(r.URL.Query()["tag_id"]),
	}
	applyTriageIndexedViewFilter(r.URL.Query().Get("view"), &filter)
	return filter
}

func (h *Handlers) listLiveMessagesWithTriage(ctx context.Context, userID, mailLogin, pass, mailbox, query string, page, pageSize int, filter models.IndexedMessageFilter) ([]mail.MessageSummary, error) {
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	scanAll := mailConversationFilterActive(filter)
	accepted := make([]mail.MessageSummary, 0, offset+pageSize)
	for scanPage := 1; scanPage <= liveTriageMaxPages; scanPage++ {
		var (
			items []mail.MessageSummary
			err   error
		)
		if strings.TrimSpace(query) != "" {
			items, err = h.svc.Mail().Search(ctx, mailLogin, pass, mailbox, query, scanPage, liveTriageScanPageSize)
		} else {
			items, err = h.svc.Mail().ListMessages(ctx, mailLogin, pass, mailbox, scanPage, liveTriageScanPageSize)
		}
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		items = presentLiveMessageSummaries(items, mailbox)
		items, err = decorateLiveMessageSummariesWithTriage(ctx, h, userID, items)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if liveSummaryShouldHideSnoozed(item, filter) {
				continue
			}
			if !liveSummaryMatchesTriageFilter(item, filter) {
				continue
			}
			accepted = append(accepted, item)
		}
		if !scanAll && len(accepted) >= offset+pageSize {
			break
		}
		if len(items) < liveTriageScanPageSize {
			break
		}
	}
	if mailConversationFilterActive(filter) {
		accepted = collapseLiveMessageSummariesByThread(accepted)
		sortLiveSummariesForTriageView(accepted, filter)
	}
	if offset >= len(accepted) {
		return []mail.MessageSummary{}, nil
	}
	end := offset + pageSize
	if end > len(accepted) {
		end = len(accepted)
	}
	return append([]mail.MessageSummary(nil), accepted[offset:end]...), nil
}

func (h *Handlers) queryIndexedMessagesWithTriage(
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
	accountIDs := indexedScopeAccountIDs(accounts)
	if len(accountIDs) == 0 {
		return []models.IndexedMessage{}, 0, nil
	}
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}
	useSearch := preferSearch || strings.TrimSpace(filter.Query) != ""
	multiAccount := len(accountIDs) > 1
	baseFilter := stripIndexedTriageFilter(filter)
	selfEmails := []string(nil)
	if filter.Waiting {
		selfEmails = indexedAccountsSelfEmails(ctx, h, u, accounts)
	}
	batchLimit := pageSize * 3
	if batchLimit < 120 {
		batchLimit = 120
	}
	rawOffset := 0
	accepted := make([]models.IndexedMessage, 0, offset+pageSize)
	for {
		var (
			items []models.IndexedMessage
			err   error
		)
		switch {
		case multiAccount && useSearch:
			items, _, err = h.svc.Store().SearchIndexedMessagesByAccounts(ctx, accountIDs, mailboxFilters, baseFilter, batchLimit, rawOffset)
		case multiAccount:
			items, _, err = h.svc.Store().ListIndexedMessagesByAccounts(ctx, accountIDs, mailboxFilters, baseFilter, sortOrder, batchLimit, rawOffset)
		case useSearch:
			items, _, err = h.svc.Store().SearchIndexedMessages(ctx, accountIDs[0], mailbox, baseFilter, batchLimit, rawOffset)
		default:
			items, _, err = h.svc.Store().ListIndexedMessages(ctx, accountIDs[0], mailbox, baseFilter, sortOrder, batchLimit, rawOffset)
		}
		if err != nil {
			return nil, 0, err
		}
		if len(items) == 0 {
			break
		}
		items, err = decorateIndexedMessagesWithTriage(ctx, h, u.ID, items)
		if err != nil {
			return nil, 0, err
		}
		for _, item := range items {
			if mailTriageShouldHideSnoozed(item.Triage, filter) {
				continue
			}
			if !mailTriageMatchesFilter(item.Triage, filter) {
				continue
			}
			accepted = append(accepted, item)
		}
		rawOffset += len(items)
		if len(items) < batchLimit {
			break
		}
	}
	if filter.Waiting {
		accepted = filterWaitingIndexedMessages(accepted, selfEmails)
	}
	if mailTriageExplicitFilterActive(filter) {
		accepted = collapseIndexedMessagesByThread(accepted)
		sortIndexedMessagesForTriageView(accepted, filter)
	}
	total := len(accepted)
	if offset >= total {
		return []models.IndexedMessage{}, total, nil
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	return append([]models.IndexedMessage(nil), accepted[offset:end]...), total, nil
}
