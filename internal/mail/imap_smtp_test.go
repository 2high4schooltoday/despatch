package mail

import (
	"bytes"
	"encoding/base64"
	"reflect"
	"strings"
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

func TestParseRawMessageSeedsReferencesFromInReplyToWithoutCorruption(t *testing.T) {
	raw := []byte(
		"From: Bob <bob@example.com>\r\n" +
			"To: user@example.com\r\n" +
			"Subject: Re: Topic\r\n" +
			"Message-ID: <reply@example.com>\r\n" +
			"In-Reply-To: <projects-parent@example.com>\r\n" +
			"\r\nBody",
	)

	msg, err := ParseRawMessage(raw, "INBOX", 1)
	if err != nil {
		t.Fatalf("ParseRawMessage: %v", err)
	}
	if msg.InReplyTo != "projects-parent@example.com" {
		t.Fatalf("expected in-reply-to to be normalized, got %q", msg.InReplyTo)
	}
	expectedRefs := []string{"projects-parent@example.com"}
	if !reflect.DeepEqual(msg.References, expectedRefs) {
		t.Fatalf("expected references to be seeded from in-reply-to without corruption, got %#v", msg.References)
	}
}

func TestParseRawMessageCarriesThreadIDMatchingLiveSummary(t *testing.T) {
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

	tests := []struct {
		name        string
		subject     string
		messageID   string
		inReplyTo   string
		references  []string
		expectedRef []string
	}{
		{
			name:        "root message id",
			subject:     "Topic",
			messageID:   "<root@example.com>",
			expectedRef: nil,
		},
		{
			name:        "reply in reply to",
			subject:     "Re: Topic",
			messageID:   "<reply-1@example.com>",
			inReplyTo:   "<root@example.com>",
			expectedRef: []string{"root@example.com"},
		},
		{
			name:        "nested references",
			subject:     "Re: Topic",
			messageID:   "<reply-2@example.com>",
			inReplyTo:   "<reply-1@example.com>",
			references:  []string{"<root@example.com>", "<reply-1@example.com>"},
			expectedRef: []string{"root@example.com", "reply-1@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{
				"From: Alice <alice@example.com>",
				"To: Bob <bob@example.com>",
				"Subject: " + tt.subject,
				"Message-ID: " + tt.messageID,
			}
			headerLines := []string{
				"Message-Id: " + tt.messageID,
			}
			if tt.inReplyTo != "" {
				lines = append(lines, "In-Reply-To: "+tt.inReplyTo)
				headerLines = append(headerLines, "In-Reply-To: "+tt.inReplyTo)
			}
			if len(tt.references) > 0 {
				referencesLine := strings.Join(tt.references, " ")
				lines = append(lines, "References: "+referencesLine)
				headerLines = append(headerLines, "References: "+referencesLine)
			}
			lines = append(lines, "", "Body")
			raw := []byte(strings.Join(lines, "\r\n"))

			msg, err := ParseRawMessage(raw, "INBOX", 7)
			if err != nil {
				t.Fatalf("ParseRawMessage: %v", err)
			}

			summary := buildMessageSummary(&imap.Message{
				Uid: 7,
				Envelope: &imap.Envelope{
					Subject:   tt.subject,
					MessageId: tt.messageID,
					InReplyTo: tt.inReplyTo,
				},
				Body: map[*imap.BodySectionName]imap.Literal{
					threadHeaderBodyKey: bytes.NewReader([]byte(strings.Join(append(headerLines, "", ""), "\r\n"))),
				},
			}, "INBOX", "Alice <alice@example.com>", tt.subject, time.Date(2026, 3, 10, 10, 0, 0, 0, time.UTC), nil, threadHeaderSection)

			if msg.ThreadID == "" {
				t.Fatalf("expected parsed message thread id to be populated")
			}
			if msg.ThreadID != summary.ThreadID {
				t.Fatalf("expected parsed thread id %q to match summary thread id %q", msg.ThreadID, summary.ThreadID)
			}
			if !reflect.DeepEqual(msg.References, tt.expectedRef) {
				t.Fatalf("expected normalized references %#v, got %#v", tt.expectedRef, msg.References)
			}
		})
	}
}

func TestParseRawMessageUsesOutlookThreadHintsWhenReplyHeadersAreMissing(t *testing.T) {
	rootBytes := make([]byte, 22)
	for i := range rootBytes {
		rootBytes[i] = byte(i + 1)
	}
	replyBytes := append(append([]byte{}, rootBytes...), []byte{23, 24, 25, 26, 27}...)
	rootIndex := base64.StdEncoding.EncodeToString(rootBytes)
	replyIndex := base64.StdEncoding.EncodeToString(replyBytes)

	rootRaw := []byte(strings.Join([]string{
		"From: Alice <alice@example.com>",
		"To: User <user@example.com>",
		"Subject: Topic",
		"Message-ID: <root@example.com>",
		"Thread-Topic: Topic",
		"Thread-Index: " + rootIndex,
		"",
		"Root body",
	}, "\r\n"))
	replyRaw := []byte(strings.Join([]string{
		"From: User <user@example.com>",
		"To: Alice <alice@example.com>",
		"Subject: Re: Topic",
		"Message-ID: <reply@example.com>",
		"Thread-Topic: Topic",
		"Thread-Index: " + replyIndex,
		"",
		"Reply body",
	}, "\r\n"))

	root, err := ParseRawMessage(rootRaw, "INBOX", 1)
	if err != nil {
		t.Fatalf("ParseRawMessage root: %v", err)
	}
	reply, err := ParseRawMessage(replyRaw, "Sent Messages", 2)
	if err != nil {
		t.Fatalf("ParseRawMessage reply: %v", err)
	}
	if root.ThreadIndex == "" || reply.ThreadIndex == "" {
		t.Fatalf("expected normalized conversation index roots, got root=%q reply=%q", root.ThreadIndex, reply.ThreadIndex)
	}
	if root.ThreadID == "" || reply.ThreadID == "" || root.ThreadID != reply.ThreadID {
		t.Fatalf("expected outlook hints to keep both messages in one thread, got root=%q reply=%q", root.ThreadID, reply.ThreadID)
	}
}

func TestMailboxPageUIDsDedupesAndKeepsNewestFirst(t *testing.T) {
	got := mailboxPageUIDs([]uint32{7, 9, 8, 9, 6, 8, 5}, 1, 4)
	want := []uint32{9, 8, 7, 6}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected first page UIDs %#v, got %#v", want, got)
	}

	secondPage := mailboxPageUIDs([]uint32{7, 9, 8, 9, 6, 8, 5}, 2, 2)
	secondWant := []uint32{7, 6}
	if !reflect.DeepEqual(secondPage, secondWant) {
		t.Fatalf("expected second page UIDs %#v, got %#v", secondWant, secondPage)
	}
}

func TestCollectedFetchedMessageSummariesCollapseDuplicateUIDs(t *testing.T) {
	messages := make(chan *imap.Message, 3)
	messages <- &imap.Message{
		Uid: 9,
		Envelope: &imap.Envelope{
			Subject: "Newest copy",
			Date:    time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC),
		},
		InternalDate: time.Date(2026, 3, 11, 9, 0, 0, 0, time.UTC),
	}
	messages <- &imap.Message{
		Uid: 9,
		Envelope: &imap.Envelope{
			Subject: "Duplicate should be ignored",
			Date:    time.Date(2026, 3, 11, 9, 1, 0, 0, time.UTC),
		},
		InternalDate: time.Date(2026, 3, 11, 9, 1, 0, 0, time.UTC),
	}
	messages <- &imap.Message{
		Uid: 8,
		Envelope: &imap.Envelope{
			Subject: "Older copy",
			Date:    time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
		},
		InternalDate: time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC),
	}
	close(messages)

	msgByUID := collectFetchedMessageSummariesByUID(messages, "INBOX", nil, nil)
	got := orderedMessageSummariesForUIDs([]uint32{9, 8, 9}, msgByUID)
	if len(got) != 2 {
		t.Fatalf("expected duplicate UID to collapse to 2 summaries, got %d", len(got))
	}
	if got[0].ID != EncodeMessageID("INBOX", 9) || got[1].ID != EncodeMessageID("INBOX", 8) {
		t.Fatalf("expected newest-first UID order, got %#v", got)
	}
	if got[0].Subject != "Newest copy" {
		t.Fatalf("expected first fetched UID copy to win, got %#v", got[0])
	}
}
