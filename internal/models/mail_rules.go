package models

import "time"

type MailRuleConditions struct {
	FromContains    string `json:"from_contains,omitempty"`
	FromDomainIs    string `json:"from_domain_is,omitempty"`
	ToContains      string `json:"to_contains,omitempty"`
	SubjectContains string `json:"subject_contains,omitempty"`
	BodyContains    string `json:"body_contains,omitempty"`
}

type MailRuleActions struct {
	MoveToMailbox string `json:"move_to_mailbox,omitempty"`
	MoveToRole    string `json:"move_to_role,omitempty"`
	MarkRead      bool   `json:"mark_read,omitempty"`
	Redirect      string `json:"redirect,omitempty"`
	Stop          bool   `json:"stop,omitempty"`
}

type MailRule struct {
	ID         string             `json:"id"`
	AccountID  string             `json:"account_id"`
	Name       string             `json:"name"`
	Enabled    bool               `json:"enabled"`
	Position   int                `json:"position"`
	MatchMode  string             `json:"match_mode"`
	Conditions MailRuleConditions `json:"conditions"`
	Actions    MailRuleActions    `json:"actions"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}
