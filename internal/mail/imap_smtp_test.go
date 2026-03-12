package mail

import (
	"bytes"
	"testing"
	"time"

	"github.com/emersion/go-imap"
)

func TestBuildMessageSummaryUsesMessageHeadersForLiveThreading(t *testing.T) {
	threadHeaderSection := &imap.BodySectionName{
		Peek: true,
		BodyPartName: imap.BodyPartName{
			Specifier: imap.HeaderSpecifier,
			Fields:    []string{"Message-Id", "In-Reply-To", "References"},
		},
	}
	threadHeaderBodyKey := &imap.BodySectionName{
		BodyPartName: imap.BodyPartName{
			Specifier: imap.HeaderSpecifier,
			Fields:    []string{"Message-Id", "In-Reply-To", "References"},
		},
	}

	root := buildMessageSummary(&imap.Message{
		Uid: 1,
		Envelope: &imap.Envelope{
			Subject:   "Topic",
			MessageId: "<root@example.com>",
		},
		Body: map[*imap.BodySectionName]imap.Literal{
			threadHeaderBodyKey: bytes.NewReader([]byte("Message-Id: <root@example.com>\r\n\r\n")),
		},
	}, "INBOX", "Alice <alice@example.com>", "Topic", time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC), nil, threadHeaderSection)

	reply := buildMessageSummary(&imap.Message{
		Uid: 2,
		Envelope: &imap.Envelope{
			Subject:   "Re: Topic",
			MessageId: "<reply-2@example.com>",
			InReplyTo: "<reply-1@example.com>",
		},
		Body: map[*imap.BodySectionName]imap.Literal{
			threadHeaderBodyKey: bytes.NewReader([]byte(
				"Message-Id: <reply-2@example.com>\r\n" +
					"In-Reply-To: <reply-1@example.com>\r\n" +
					"References: <root@example.com> <reply-1@example.com>\r\n\r\n",
			)),
		},
	}, "INBOX", "Bob <bob@example.com>", "Re: Topic", time.Date(2026, 3, 10, 10, 5, 0, 0, time.UTC), nil, threadHeaderSection)

	if root.ThreadID == "" || reply.ThreadID == "" {
		t.Fatalf("expected live thread ids to be populated, got root=%q reply=%q", root.ThreadID, reply.ThreadID)
	}
	if root.ThreadID != reply.ThreadID {
		t.Fatalf("expected references-based live threading to keep conversation together, got root=%q reply=%q", root.ThreadID, reply.ThreadID)
	}
}
