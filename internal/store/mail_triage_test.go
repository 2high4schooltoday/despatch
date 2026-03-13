package store

import (
	"context"
	"testing"
	"time"

	"despatch/internal/models"
)

func TestApplyMailThreadTriageSupportsCreateOnTypeAndClearLifecycle(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()

	user, err := st.CreateUser(ctx, "triage@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	targetA := models.MailTriageTarget{
		Source:   "live",
		ThreadID: "thread-a",
		Mailbox:  "INBOX",
		Subject:  "Budget review",
		From:     "alice@example.com",
	}
	targetB := models.MailTriageTarget{
		Source:   "live",
		ThreadID: "thread-b",
		Mailbox:  "INBOX",
		Subject:  "Vendor invoice",
		From:     "bob@example.com",
	}
	snoozedUntil := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	reminderAt := time.Now().Add(45 * time.Minute).UTC().Truncate(time.Second)

	states, err := st.ApplyMailThreadTriage(ctx, user.ID, []models.MailTriageTarget{targetA}, models.MailTriageMutation{
		SnoozedUntil: &snoozedUntil,
		ReminderAt:   &reminderAt,
		CategoryName: "Finance",
		AddTagNames:  []string{"urgent", "billing", "urgent"},
	})
	if err != nil {
		t.Fatalf("apply triage: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 triage state, got %d", len(states))
	}
	stateA := states[0]
	if stateA.TriageKey == "" {
		t.Fatalf("expected triage key to be populated")
	}
	if stateA.Triage.Category == nil || stateA.Triage.Category.Name != "Finance" {
		t.Fatalf("expected Finance category, got %#v", stateA.Triage.Category)
	}
	if !stateA.Triage.IsSnoozed {
		t.Fatalf("expected thread to be snoozed")
	}
	if stateA.Triage.IsFollowUpDue {
		t.Fatalf("expected reminder to be scheduled in the future")
	}
	if stateA.Triage.SnoozedUntil == nil || !stateA.Triage.SnoozedUntil.UTC().Equal(snoozedUntil) {
		t.Fatalf("unexpected snooze value: %#v", stateA.Triage.SnoozedUntil)
	}
	if stateA.Triage.ReminderAt == nil || !stateA.Triage.ReminderAt.UTC().Equal(reminderAt) {
		t.Fatalf("unexpected reminder value: %#v", stateA.Triage.ReminderAt)
	}
	if len(stateA.Triage.Tags) != 2 {
		t.Fatalf("expected 2 unique tags, got %#v", stateA.Triage.Tags)
	}
	gotTags := map[string]bool{}
	for _, tag := range stateA.Triage.Tags {
		gotTags[tag.Name] = true
	}
	if !gotTags["billing"] || !gotTags["urgent"] {
		t.Fatalf("unexpected tags: %#v", stateA.Triage.Tags)
	}

	categories, err := st.ListMailTriageCategories(ctx, user.ID)
	if err != nil {
		t.Fatalf("list categories: %v", err)
	}
	if len(categories) != 1 || categories[0].Name != "Finance" {
		t.Fatalf("unexpected categories: %#v", categories)
	}
	tags, err := st.ListMailTriageTags(ctx, user.ID)
	if err != nil {
		t.Fatalf("list tags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 catalog tags, got %#v", tags)
	}

	states, err = st.ApplyMailThreadTriage(ctx, user.ID, []models.MailTriageTarget{targetB}, models.MailTriageMutation{
		CategoryName: " finance ",
		AddTagNames:  []string{"URGENT"},
	})
	if err != nil {
		t.Fatalf("apply triage with existing names: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("expected 1 triage state for second thread, got %d", len(states))
	}
	if states[0].Triage.Category == nil || states[0].Triage.Category.Name != "Finance" {
		t.Fatalf("expected existing Finance category to be reused, got %#v", states[0].Triage.Category)
	}
	if len(states[0].Triage.Tags) != 1 || states[0].Triage.Tags[0].Name != "urgent" {
		t.Fatalf("expected existing urgent tag to be reused, got %#v", states[0].Triage.Tags)
	}

	categories, err = st.ListMailTriageCategories(ctx, user.ID)
	if err != nil {
		t.Fatalf("list categories after reuse: %v", err)
	}
	if len(categories) != 1 {
		t.Fatalf("expected category catalog to stay deduped, got %#v", categories)
	}
	tags, err = st.ListMailTriageTags(ctx, user.ID)
	if err != nil {
		t.Fatalf("list tags after reuse: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected tag catalog to stay deduped, got %#v", tags)
	}

	cleared, err := st.ApplyMailThreadTriage(ctx, user.ID, []models.MailTriageTarget{targetA}, models.MailTriageMutation{
		ClearSnooze:   true,
		ClearReminder: true,
		ClearCategory: true,
		ClearTags:     true,
	})
	if err != nil {
		t.Fatalf("clear triage: %v", err)
	}
	if len(cleared) != 1 {
		t.Fatalf("expected 1 cleared triage state, got %d", len(cleared))
	}
	stateA = cleared[0]
	if stateA.Triage.Category != nil {
		t.Fatalf("expected cleared category, got %#v", stateA.Triage.Category)
	}
	if len(stateA.Triage.Tags) != 0 {
		t.Fatalf("expected cleared tags, got %#v", stateA.Triage.Tags)
	}
	if stateA.Triage.SnoozedUntil != nil || stateA.Triage.ReminderAt != nil {
		t.Fatalf("expected cleared snooze/reminder, got %#v", stateA.Triage)
	}
	if stateA.Triage.IsSnoozed || stateA.Triage.IsFollowUpDue {
		t.Fatalf("expected cleared triage booleans, got %#v", stateA.Triage)
	}
}

func TestPollDueMailTriageRemindersMarksEachScheduledReminderOnce(t *testing.T) {
	st := newV2ScopedStore(t)
	ctx := context.Background()

	user, err := st.CreateUser(ctx, "triage-reminders@example.com", "hash", "user", models.UserActive)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	target := models.MailTriageTarget{
		Source:   "live",
		ThreadID: "thread-reminder",
		Mailbox:  "INBOX",
		Subject:  "Follow up budget review",
		From:     "alice@example.com",
	}
	firstReminder := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Second)
	if _, err := st.ApplyMailThreadTriage(ctx, user.ID, []models.MailTriageTarget{target}, models.MailTriageMutation{
		ReminderAt: &firstReminder,
	}); err != nil {
		t.Fatalf("apply first reminder: %v", err)
	}

	firstPoll, err := st.PollDueMailTriageReminders(ctx, user.ID, 10)
	if err != nil {
		t.Fatalf("first poll: %v", err)
	}
	if len(firstPoll) != 1 {
		t.Fatalf("expected 1 due reminder on first poll, got %#v", firstPoll)
	}
	if firstPoll[0].ThreadID != target.ThreadID || !firstPoll[0].ReminderAt.Equal(firstReminder) {
		t.Fatalf("unexpected first reminder payload: %#v", firstPoll[0])
	}

	secondPoll, err := st.PollDueMailTriageReminders(ctx, user.ID, 10)
	if err != nil {
		t.Fatalf("second poll: %v", err)
	}
	if len(secondPoll) != 0 {
		t.Fatalf("expected no duplicate reminder on second poll, got %#v", secondPoll)
	}

	secondReminder := time.Now().Add(-30 * time.Second).UTC().Truncate(time.Second)
	if _, err := st.ApplyMailThreadTriage(ctx, user.ID, []models.MailTriageTarget{target}, models.MailTriageMutation{
		ReminderAt: &secondReminder,
	}); err != nil {
		t.Fatalf("reschedule reminder: %v", err)
	}

	thirdPoll, err := st.PollDueMailTriageReminders(ctx, user.ID, 10)
	if err != nil {
		t.Fatalf("third poll: %v", err)
	}
	if len(thirdPoll) != 1 {
		t.Fatalf("expected rescheduled reminder to emit once, got %#v", thirdPoll)
	}
	if !thirdPoll[0].ReminderAt.Equal(secondReminder) {
		t.Fatalf("expected new scheduled timestamp, got %#v", thirdPoll[0])
	}
}
