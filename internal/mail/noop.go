package mail

import (
	"context"
	"fmt"
	"io"
	"time"
)

type NoopClient struct{}

func (NoopClient) ListMailboxes(ctx context.Context, user, pass string) ([]Mailbox, error) {
	return []Mailbox{
		{Name: "INBOX", Unread: 0, Messages: 0},
		{Name: "Sent", Unread: 0, Messages: 0},
		{Name: "Drafts", Unread: 0, Messages: 0},
		{Name: "Trash", Unread: 0, Messages: 0},
	}, nil
}

func (NoopClient) ListMessages(ctx context.Context, user, pass, mailbox string, page, pageSize int) ([]MessageSummary, error) {
	return []MessageSummary{}, nil
}

func (NoopClient) GetMessage(ctx context.Context, user, pass, id string) (Message, error) {
	return Message{ID: id, Subject: "Noop", Date: time.Now().UTC()}, fmt.Errorf("message not found")
}

func (NoopClient) Search(ctx context.Context, user, pass, mailbox, query string, page, pageSize int) ([]MessageSummary, error) {
	return []MessageSummary{}, nil
}

func (NoopClient) Send(ctx context.Context, user, pass string, req SendRequest) (SendResult, error) {
	return SendResult{SavedCopy: true, SavedCopyMailbox: "Sent"}, nil
}

func (NoopClient) SetFlags(ctx context.Context, user, pass, id string, flags []string) error {
	return nil
}
func (NoopClient) UpdateFlags(ctx context.Context, user, pass, id string, patch FlagPatch) error {
	return nil
}
func (NoopClient) Move(ctx context.Context, user, pass, id, mailbox string) error { return nil }

func (NoopClient) GetAttachment(ctx context.Context, user, pass, attachmentID string) (AttachmentContent, error) {
	return AttachmentContent{}, fmt.Errorf("attachment not found")
}

func (NoopClient) GetAttachmentStream(ctx context.Context, user, pass, attachmentID string) (AttachmentMeta, io.ReadCloser, error) {
	return AttachmentMeta{}, nil, fmt.Errorf("attachment not found")
}
