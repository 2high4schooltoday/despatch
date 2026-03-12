package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"despatch/internal/mail"
)

const settingIndexedThreadHeadersRepaired = "mail_index_thread_headers_v1"

// EnsureIndexedThreadHeadersRepaired performs a one-time repair pass for
// historical indexed rows whose stored thread ids predate header-based
// conversation grouping.
func (s *Store) EnsureIndexedThreadHeadersRepaired(ctx context.Context) error {
	done, ok, err := s.GetSetting(ctx, settingIndexedThreadHeadersRepaired)
	if err == nil && ok && strings.EqualFold(strings.TrimSpace(done), "done") {
		return nil
	}
	if err != nil {
		return err
	}
	if !s.tableExists(ctx, "message_index") {
		return nil
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`)
	}()

	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()

	now := time.Now().UTC()
	rows, err := conn.QueryContext(ctx,
		`SELECT account_id,id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,subject,from_value,raw_source
		 FROM message_index
		 ORDER BY account_id ASC, internal_date ASC, date_header ASC, created_at ASC, id ASC`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rowUpdate struct {
		accountID        string
		id               string
		threadID         string
		messageIDHeader  string
		inReplyToHeader  string
		referencesHeader string
	}

	headerToThread := map[string]map[string]string{}
	updates := make([]rowUpdate, 0, 64)
	touchedAccounts := map[string]struct{}{}

	for rows.Next() {
		var (
			accountID        string
			id               string
			mailbox          string
			uid              uint32
			threadID         string
			messageIDHeader  string
			inReplyToHeader  string
			referencesHeader string
			subject          string
			fromValue        string
			rawSource        string
		)
		if err := rows.Scan(
			&accountID,
			&id,
			&mailbox,
			&uid,
			&threadID,
			&messageIDHeader,
			&inReplyToHeader,
			&referencesHeader,
			&subject,
			&fromValue,
			&rawSource,
		); err != nil {
			return err
		}

		normalizedMessageID := mail.NormalizeMessageIDHeader(messageIDHeader)
		normalizedInReplyTo := mail.NormalizeMessageIDHeader(inReplyToHeader)
		normalizedReferences := mail.ParseMessageIDList(referencesHeader)

		if (normalizedMessageID == "" || (normalizedInReplyTo == "" && len(normalizedReferences) == 0)) && strings.TrimSpace(rawSource) != "" {
			if parsed, parseErr := mail.ParseRawMessage([]byte(rawSource), mailbox, uid); parseErr == nil {
				if normalizedMessageID == "" {
					normalizedMessageID = mail.NormalizeMessageIDHeader(parsed.MessageID)
				}
				if normalizedInReplyTo == "" {
					normalizedInReplyTo = mail.NormalizeMessageIDHeader(parsed.InReplyTo)
				}
				if len(normalizedReferences) == 0 {
					normalizedReferences = mail.NormalizeMessageIDHeaders(parsed.References)
				}
			}
		}

		accountHeaders := headerToThread[accountID]
		if accountHeaders == nil {
			accountHeaders = map[string]string{}
			headerToThread[accountID] = accountHeaders
		}
		nextThreadID := resolveRepairedIndexedThreadID(accountID, normalizedMessageID, normalizedInReplyTo, normalizedReferences, subject, fromValue, accountHeaders)
		if normalizedMessageID != "" {
			accountHeaders[normalizedMessageID] = nextThreadID
		}

		currentThreadID := mail.NormalizeIndexedThreadID(accountID, threadID)
		nextReferencesHeader := mail.FormatMessageIDList(normalizedReferences)
		if currentThreadID == nextThreadID &&
			mail.NormalizeMessageIDHeader(messageIDHeader) == normalizedMessageID &&
			mail.NormalizeMessageIDHeader(inReplyToHeader) == normalizedInReplyTo &&
			mail.FormatMessageIDList(mail.ParseMessageIDList(referencesHeader)) == nextReferencesHeader {
			continue
		}

		updates = append(updates, rowUpdate{
			accountID:        accountID,
			id:               id,
			threadID:         nextThreadID,
			messageIDHeader:  normalizedMessageID,
			inReplyToHeader:  normalizedInReplyTo,
			referencesHeader: nextReferencesHeader,
		})
		touchedAccounts[accountID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, item := range updates {
		if _, err := conn.ExecContext(ctx,
			`UPDATE message_index
			 SET thread_id=?, message_id_header=?, in_reply_to_header=?, references_header=?, updated_at=?
			 WHERE account_id=? AND id=?`,
			item.threadID,
			item.messageIDHeader,
			item.inReplyToHeader,
			item.referencesHeader,
			now,
			item.accountID,
			item.id,
		); err != nil {
			return err
		}
	}

	accountIDs := make([]string, 0, len(touchedAccounts))
	for accountID := range touchedAccounts {
		accountIDs = append(accountIDs, accountID)
	}
	sort.Strings(accountIDs)
	for _, accountID := range accountIDs {
		if err := rebuildThreadIndexOnConn(ctx, conn, accountID, now); err != nil {
			return err
		}
	}

	if _, err := conn.ExecContext(ctx,
		`INSERT INTO settings(key,value,updated_at)
		 VALUES(?,?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		settingIndexedThreadHeadersRepaired,
		"done",
		now,
	); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}

func resolveRepairedIndexedThreadID(accountID, messageID, inReplyTo string, references []string, subject, from string, headerToThread map[string]string) string {
	for _, item := range mail.NormalizeMessageIDHeaders(references) {
		if threadID := strings.TrimSpace(headerToThread[item]); threadID != "" {
			return threadID
		}
	}
	if normalized := mail.NormalizeMessageIDHeader(inReplyTo); normalized != "" {
		if threadID := strings.TrimSpace(headerToThread[normalized]); threadID != "" {
			return threadID
		}
	}
	return mail.NormalizeIndexedThreadID(accountID, mail.DeriveIndexedThreadID(messageID, inReplyTo, references, subject, from))
}

func rebuildThreadIndexOnConn(ctx context.Context, conn *sql.Conn, accountID string, now time.Time) error {
	rows, err := conn.QueryContext(ctx,
		`SELECT id,mailbox,thread_id,subject,from_value,seen,has_attachments,flagged,importance,internal_date
		 FROM message_index
		 WHERE account_id=?
		 ORDER BY internal_date DESC`,
		accountID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type threadAgg struct {
		ID             string
		Mailbox        string
		SubjectNorm    string
		Participants   map[string]struct{}
		MessageCount   int
		UnreadCount    int
		HasAttachments bool
		HasFlagged     bool
		Importance     int
		LatestMessage  string
		LatestAt       time.Time
	}

	threads := make(map[string]*threadAgg, 64)
	for rows.Next() {
		var (
			id           string
			mailbox      string
			threadID     string
			subject      string
			fromValue    string
			seenInt      int
			attachInt    int
			flaggedInt   int
			importance   int
			internalDate time.Time
		)
		if err := rows.Scan(&id, &mailbox, &threadID, &subject, &fromValue, &seenInt, &attachInt, &flaggedInt, &importance, &internalDate); err != nil {
			return err
		}
		scopedMessageID := mail.NormalizeIndexedMessageID(accountID, id)
		scopedThreadID := mail.NormalizeIndexedThreadID(accountID, threadID)
		agg := threads[scopedThreadID]
		if agg == nil {
			agg = &threadAgg{
				ID:           scopedThreadID,
				Mailbox:      mailbox,
				SubjectNorm:  firstNonEmptyString(strings.TrimSpace(subject), "(no subject)"),
				Participants: map[string]struct{}{},
			}
			threads[scopedThreadID] = agg
		}
		agg.MessageCount++
		if seenInt == 0 {
			agg.UnreadCount++
		}
		if attachInt == 1 {
			agg.HasAttachments = true
		}
		if flaggedInt == 1 {
			agg.HasFlagged = true
		}
		if importance > agg.Importance {
			agg.Importance = importance
		}
		if from := strings.TrimSpace(fromValue); from != "" {
			agg.Participants[from] = struct{}{}
		}
		if agg.LatestAt.IsZero() || internalDate.After(agg.LatestAt) {
			agg.LatestAt = internalDate
			agg.LatestMessage = scopedMessageID
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, `DELETE FROM thread_index WHERE account_id=?`, accountID); err != nil {
		return err
	}
	for _, agg := range threads {
		participants := make([]string, 0, len(agg.Participants))
		for participant := range agg.Participants {
			participants = append(participants, participant)
		}
		sort.Strings(participants)
		participantsJSON, _ := json.Marshal(participants)
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO thread_index(
			  id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			agg.ID,
			accountID,
			agg.Mailbox,
			agg.SubjectNorm,
			string(participantsJSON),
			agg.MessageCount,
			agg.UnreadCount,
			boolToInt(agg.HasAttachments),
			boolToInt(agg.HasFlagged),
			agg.Importance,
			agg.LatestMessage,
			agg.LatestAt,
			now,
		); err != nil {
			return err
		}
	}
	return nil
}
