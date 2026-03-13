package mail

import "time"

type TriageCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TriageTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type TriageState struct {
	SnoozedUntil  *time.Time       `json:"snoozed_until"`
	ReminderAt    *time.Time       `json:"reminder_at"`
	Category      *TriageCategory  `json:"category"`
	Tags          []TriageTag      `json:"tags"`
	IsSnoozed     bool             `json:"is_snoozed"`
	IsFollowUpDue bool             `json:"is_follow_up_due"`
}

func DefaultTriageState() TriageState {
	return TriageState{
		Tags: []TriageTag{},
	}
}
