package mail

import (
	"context"
	"errors"
	"io"
	"time"
)

type Mailbox struct {
	Name      string `json:"name"`
	Role      string `json:"role,omitempty"`
	Unread    int    `json:"unread"`
	Messages  int    `json:"messages"`
	CanRename bool   `json:"can_rename,omitempty"`
	CanDelete bool   `json:"can_delete,omitempty"`
}

type MessageSummary struct {
	ID        string    `json:"id"`
	AccountID string    `json:"account_id,omitempty"`
	Mailbox   string    `json:"mailbox,omitempty"`
	Source    string    `json:"source,omitempty"`
	From      string    `json:"from"`
	Subject   string    `json:"subject"`
	Date      time.Time `json:"date"`
	Seen      bool      `json:"seen"`
	Flagged   bool      `json:"flagged,omitempty"`
	Answered  bool      `json:"answered,omitempty"`
	Preview   string    `json:"preview,omitempty"`
	ThreadID  string    `json:"thread_id,omitempty"`
	TriageKey string    `json:"triage_key,omitempty"`
	Triage    TriageState `json:"triage"`
}

type AttachmentMeta struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	Inline      bool   `json:"inline,omitempty"`
	ContentID   string `json:"content_id,omitempty"`
}

type Message struct {
	ID          string           `json:"id"`
	Mailbox     string           `json:"mailbox"`
	Source      string           `json:"source,omitempty"`
	UID         uint32           `json:"uid"`
	ThreadID    string           `json:"thread_id,omitempty"`
	From        string           `json:"from"`
	To          []string         `json:"to"`
	CC          []string         `json:"cc,omitempty"`
	BCC         []string         `json:"bcc,omitempty"`
	Subject     string           `json:"subject"`
	Date        time.Time        `json:"date"`
	Seen        bool             `json:"seen"`
	Flagged     bool             `json:"flagged,omitempty"`
	Answered    bool             `json:"answered,omitempty"`
	Body        string           `json:"body"`
	BodyHTML    string           `json:"body_html,omitempty"`
	Attachments []AttachmentMeta `json:"attachments"`
	TriageKey   string           `json:"triage_key,omitempty"`
	Triage      TriageState      `json:"triage"`
	MessageID   string           `json:"-"`
	InReplyTo   string           `json:"-"`
	References  []string         `json:"-"`
}

type MailboxSnapshot struct {
	Mailbox     Mailbox
	UIDNext     uint32
	UIDValidity uint32
}

type SyncMessage struct {
	Mailbox      string
	UID          uint32
	Raw          []byte
	Flags        []string
	InternalDate time.Time
}

type AttachmentContent struct {
	Meta AttachmentMeta
	Data []byte
}

type Quota struct {
	UsedBytes     int64
	TotalBytes    int64
	UsedMessages  int64
	TotalMessages int64
}

var ErrQuotaUnsupported = errors.New("quota unsupported")

type QuotaReader interface {
	GetQuota(ctx context.Context, user, pass string) (Quota, error)
}

type SendAttachment struct {
	Filename    string
	ContentType string
	Data        []byte
	Inline      bool
	ContentID   string
}

type SendRequest struct {
	From            string           `json:"from,omitempty"`
	HeaderFromName  string           `json:"-"`
	HeaderFromEmail string           `json:"-"`
	EnvelopeFrom    string           `json:"-"`
	ReplyTo         string           `json:"-"`
	To              []string         `json:"to"`
	CC              []string         `json:"cc,omitempty"`
	BCC             []string         `json:"bcc,omitempty"`
	Subject         string           `json:"subject"`
	Body            string           `json:"body"`
	BodyHTML        string           `json:"body_html,omitempty"`
	InReplyToID     string           `json:"in_reply_to_id,omitempty"`
	References      []string         `json:"-"`
	MessageID       string           `json:"-"`
	Attachments     []SendAttachment `json:"-"`
	SentMailbox     string           `json:"-"`
}

type SendResult struct {
	SavedCopy        bool   `json:"saved_copy"`
	SavedCopyMailbox string `json:"saved_copy_mailbox,omitempty"`
	Warning          string `json:"warning,omitempty"`
}

type FlagPatch struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

type Client interface {
	ListMailboxes(ctx context.Context, user, pass string) ([]Mailbox, error)
	CreateMailbox(ctx context.Context, user, pass, mailbox string) error
	RenameMailbox(ctx context.Context, user, pass, mailbox, newMailbox string) error
	DeleteMailbox(ctx context.Context, user, pass, mailbox string) error
	ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]MessageSummary, error)
	GetMessage(ctx context.Context, user, pass, id string) (Message, error)
	GetRawMessage(ctx context.Context, user, pass, id string) ([]byte, error)
	Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]MessageSummary, error)
	Send(ctx context.Context, user, pass string, req SendRequest) (SendResult, error)
	SetFlags(ctx context.Context, user, pass, id string, flags []string) error
	UpdateFlags(ctx context.Context, user, pass, id string, patch FlagPatch) error
	Move(ctx context.Context, user, pass, id, mailbox string) error
	GetAttachment(ctx context.Context, user, pass, attachmentID string) (AttachmentContent, error)
	GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (AttachmentMeta, io.ReadCloser, error)
}
