package mail

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
)

func ParseRawMessage(raw []byte, mailbox string, uid uint32) (Message, error) {
	return parseMessage(raw, EncodeMessageID(mailbox, uid), mailbox, uid)
}

func (c *IMAPSMTPClient) ListMailboxSnapshots(ctx context.Context, user, pass string) ([]MailboxSnapshot, error) {
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	return c.listMailboxSnapshotsWithClient(cli, true)
}

func (c *IMAPSMTPClient) listMailboxSnapshotsWithClient(cli *imapclient.Client, includeCounts bool) ([]MailboxSnapshot, error) {
	ch := make(chan *imap.MailboxInfo, 32)
	done := make(chan error, 1)
	go func() {
		done <- cli.List("", "*", ch)
	}()

	statusItems := []imap.StatusItem{imap.StatusUidNext, imap.StatusUidValidity}
	if includeCounts {
		statusItems = append(statusItems, imap.StatusMessages, imap.StatusUnseen)
	}

	var out []MailboxSnapshot
	for mb := range ch {
		item := MailboxSnapshot{
			Mailbox: Mailbox{
				Name: mb.Name,
				Role: MailboxRole(mb.Name, mb.Attributes),
			},
		}
		status, err := cli.Status(mb.Name, statusItems)
		if err == nil && status != nil {
			item.UIDNext = status.UidNext
			item.UIDValidity = status.UidValidity
			if includeCounts {
				item.Mailbox.Unread = int(status.Unseen)
				item.Mailbox.Messages = int(status.Messages)
			}
		}
		out = append(out, item)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	if len(out) == 0 {
		out = []MailboxSnapshot{{
			Mailbox: Mailbox{Name: "INBOX", Role: "inbox"},
		}}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Mailbox.Role == "inbox" || out[i].Mailbox.Name == "INBOX" {
			return true
		}
		if out[j].Mailbox.Role == "inbox" || out[j].Mailbox.Name == "INBOX" {
			return false
		}
		if out[i].Mailbox.Role != out[j].Mailbox.Role {
			return out[i].Mailbox.Role < out[j].Mailbox.Role
		}
		return out[i].Mailbox.Name < out[j].Mailbox.Name
	})
	return out, nil
}

func (c *IMAPSMTPClient) ListRecentUIDs(ctx context.Context, user, pass, mailbox string, limit int) ([]uint32, error) {
	if limit <= 0 {
		return []uint32{}, nil
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()

	mbox, err := cli.Select(mailbox, true)
	if err != nil {
		return nil, err
	}
	if mbox.Messages == 0 {
		return []uint32{}, nil
	}

	total := int(mbox.Messages)
	end := total
	start := end - limit + 1
	if start < 1 {
		start = 1
	}

	seq := new(imap.SeqSet)
	seq.AddRange(uint32(start), uint32(end))
	messages := make(chan *imap.Message, end-start+1)
	done := make(chan error, 1)
	go func() {
		done <- cli.Fetch(seq, []imap.FetchItem{imap.FetchUid}, messages)
	}()

	uids := make([]uint32, 0, end-start+1)
	for msg := range messages {
		if msg == nil || msg.Uid == 0 {
			continue
		}
		uids = append(uids, msg.Uid)
	}
	if err := <-done; err != nil {
		return nil, err
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] > uids[j] })
	return uids, nil
}

func (c *IMAPSMTPClient) FetchSyncMessagesByUIDs(ctx context.Context, user, pass, mailbox string, uids []uint32) ([]SyncMessage, error) {
	if len(uids) == 0 {
		return []SyncMessage{}, nil
	}
	cli, err := c.connectIMAP(ctx, user, pass)
	if err != nil {
		return nil, err
	}
	defer cli.Logout()
	if _, err := cli.Select(mailbox, true); err != nil {
		return nil, err
	}

	seq := new(imap.SeqSet)
	for _, uid := range uids {
		if uid == 0 {
			continue
		}
		seq.AddNum(uid)
	}
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchUid, imap.FetchFlags, imap.FetchInternalDate, section.FetchItem()}
	messages := make(chan *imap.Message, len(uids))
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(seq, items, messages)
	}()

	out := make([]SyncMessage, 0, len(uids))
	for msg := range messages {
		if msg == nil || msg.Uid == 0 {
			continue
		}
		body := msg.GetBody(section)
		if body == nil {
			continue
		}
		raw, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		out = append(out, SyncMessage{
			Mailbox:      strings.TrimSpace(mailbox),
			UID:          msg.Uid,
			Raw:          raw,
			Flags:        append([]string(nil), msg.Flags...),
			InternalDate: msg.InternalDate,
		})
	}
	if err := <-done; err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID > out[j].UID })
	return out, nil
}

func mailboxSnapshotNames(items []MailboxSnapshot) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Mailbox.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out
}

func (c *IMAPSMTPClient) String() string {
	return fmt.Sprintf("imap_smtp_client(%s:%d)", c.cfg.IMAPHost, c.cfg.IMAPPort)
}
