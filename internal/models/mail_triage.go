package models

import "time"

type MailTriageCategory struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MailTriageTag struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MailTriageCategoryRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type MailTriageTagRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type MailTriageState struct {
	SnoozedUntil  *time.Time             `json:"snoozed_until"`
	ReminderAt    *time.Time             `json:"reminder_at"`
	Category      *MailTriageCategoryRef `json:"category"`
	Tags          []MailTriageTagRef     `json:"tags"`
	IsSnoozed     bool                   `json:"is_snoozed"`
	IsFollowUpDue bool                   `json:"is_follow_up_due"`
}

type MailTriageTarget struct {
	Source    string `json:"source"`
	AccountID string `json:"account_id"`
	ThreadID  string `json:"thread_id"`
	Mailbox   string `json:"mailbox,omitempty"`
	Subject   string `json:"subject,omitempty"`
	From      string `json:"from,omitempty"`
}

type MailTriageMutation struct {
	SnoozedUntil   *time.Time `json:"snoozed_until,omitempty"`
	ClearSnooze    bool       `json:"clear_snooze,omitempty"`
	ReminderAt     *time.Time `json:"reminder_at,omitempty"`
	ClearReminder  bool       `json:"clear_reminder,omitempty"`
	CategoryID     string     `json:"category_id,omitempty"`
	CategoryName   string     `json:"category_name,omitempty"`
	ClearCategory  bool       `json:"clear_category,omitempty"`
	AddTagIDs      []string   `json:"add_tag_ids,omitempty"`
	AddTagNames    []string   `json:"add_tag_names,omitempty"`
	RemoveTagIDs   []string   `json:"remove_tag_ids,omitempty"`
	RemoveTagNames []string   `json:"remove_tag_names,omitempty"`
	ClearTags      bool       `json:"clear_tags,omitempty"`
}

type MailThreadTriageRecord struct {
	TriageKey              string    `json:"triage_key"`
	UserID                 string    `json:"user_id"`
	Source                 string    `json:"source"`
	AccountID              string    `json:"account_id"`
	ThreadID               string    `json:"thread_id"`
	Mailbox                string    `json:"mailbox,omitempty"`
	Subject                string    `json:"subject,omitempty"`
	FromValue              string    `json:"from,omitempty"`
	CategoryID             string    `json:"category_id,omitempty"`
	SnoozedUntil           time.Time `json:"snoozed_until,omitempty"`
	ReminderAt             time.Time `json:"reminder_at,omitempty"`
	LastReminderNotifiedAt time.Time `json:"last_reminder_notified_at,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type MailThreadTriageState struct {
	Target    MailTriageTarget `json:"target"`
	TriageKey string           `json:"triage_key"`
	Triage    MailTriageState  `json:"triage"`
}

type MailTriageReminder struct {
	TriageKey string    `json:"triage_key"`
	Source    string    `json:"source"`
	AccountID string    `json:"account_id"`
	ThreadID  string    `json:"thread_id"`
	Mailbox   string    `json:"mailbox,omitempty"`
	Subject   string    `json:"subject,omitempty"`
	From      string    `json:"from,omitempty"`
	ReminderAt time.Time `json:"reminder_at"`
}

func DefaultMailTriageState() MailTriageState {
	return MailTriageState{
		Tags: []MailTriageTagRef{},
	}
}
