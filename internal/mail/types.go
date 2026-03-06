package mail

import (
	"context"
	"io"
	"time"
)

type Mailbox struct {
	Name     string `json:"name"`
	Unread   int    `json:"unread"`
	Messages int    `json:"messages"`
}

type MessageSummary struct {
	ID      string    `json:"id"`
	From    string    `json:"from"`
	Subject string    `json:"subject"`
	Date    time.Time `json:"date"`
	Seen    bool      `json:"seen"`
}

type AttachmentMeta struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

type Message struct {
	ID          string           `json:"id"`
	Mailbox     string           `json:"mailbox"`
	UID         uint32           `json:"uid"`
	From        string           `json:"from"`
	To          []string         `json:"to"`
	Subject     string           `json:"subject"`
	Date        time.Time        `json:"date"`
	Body        string           `json:"body"`
	Attachments []AttachmentMeta `json:"attachments"`
}

type AttachmentContent struct {
	Meta AttachmentMeta
	Data []byte
}

type SendAttachment struct {
	Filename    string
	ContentType string
	Data        []byte
	Inline      bool
	ContentID   string
}

type SendRequest struct {
	From        string           `json:"from"`
	To          []string         `json:"to"`
	CC          []string         `json:"cc,omitempty"`
	BCC         []string         `json:"bcc,omitempty"`
	Subject     string           `json:"subject"`
	Body        string           `json:"body"`
	BodyHTML    string           `json:"body_html,omitempty"`
	InReplyToID string           `json:"in_reply_to_id,omitempty"`
	Attachments []SendAttachment `json:"-"`
}

type Client interface {
	ListMailboxes(ctx context.Context, user, pass string) ([]Mailbox, error)
	ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]MessageSummary, error)
	GetMessage(ctx context.Context, user, pass, id string) (Message, error)
	Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]MessageSummary, error)
	Send(ctx context.Context, user, pass string, req SendRequest) error
	SetFlags(ctx context.Context, user, pass, id string, flags []string) error
	Move(ctx context.Context, user, pass, id, mailbox string) error
	GetAttachment(ctx context.Context, user, pass, attachmentID string) (AttachmentContent, error)
	GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (AttachmentMeta, io.ReadCloser, error)
}
