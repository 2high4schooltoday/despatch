package models

import "time"

type MailSnippet struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Subject   string    `json:"subject,omitempty"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MailFavorite struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Label         string    `json:"label"`
	AccountID     string    `json:"account_id,omitempty"`
	AccountScope  string    `json:"account_scope,omitempty"`
	Mailbox       string    `json:"mailbox,omitempty"`
	SmartView     string    `json:"smart_view,omitempty"`
	SavedSearchID string    `json:"saved_search_id,omitempty"`
	Sender        string    `json:"sender,omitempty"`
	Domain        string    `json:"domain,omitempty"`
	ThreadID      string    `json:"thread_id,omitempty"`
	MessageID     string    `json:"message_id,omitempty"`
	Subject       string    `json:"subject,omitempty"`
	From          string    `json:"from,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
