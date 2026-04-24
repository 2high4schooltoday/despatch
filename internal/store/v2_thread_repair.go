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

const settingIndexedThreadHeadersRepaired = "mail_index_thread_headers_v2"

type repairThreadRow struct {
	accountID         string
	id                string
	subject           string
	fromValue         string
	originalThreadID  string
	threadID          string
	originalMessageID string
	messageIDHeader   string
	originalInReplyTo string
	inReplyToHeader   string
	originalRefs      string
	referencesHeader  string
	references        []string
}

type accountRepairIndex struct {
	messageRows map[string]*repairThreadRow
	memo        map[string]string
	resolving   map[string]bool
}

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
	touchedThreadIDsByAccount, err := repairIndexedThreadHeadersOnConn(ctx, conn, "", now)
	if err != nil {
		return err
	}
	accountIDs := make([]string, 0, len(touchedThreadIDsByAccount))
	for accountID := range touchedThreadIDsByAccount {
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

// RepairIndexedThreadHeadersByAccount re-runs indexed thread/header repair for
// one account. This is used after full rebuild/reindex paths where mailbox or
// batch ordering may have temporarily indexed descendants before their parents.
func (s *Store) RepairIndexedThreadHeadersByAccount(ctx context.Context, accountID string) error {
	_, err := s.RepairIndexedThreadHeadersByAccountWithTouchedThreads(ctx, accountID)
	return err
}

func (s *Store) RepairIndexedThreadHeadersByAccountWithTouchedThreads(ctx context.Context, accountID string) ([]string, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, nil
	}
	if !s.tableExists(ctx, "message_index") {
		return nil, nil
	}

	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return nil, err
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`)
	}()

	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()

	now := time.Now().UTC()
	touchedThreadIDsByAccount, err := repairIndexedThreadHeadersOnConn(ctx, conn, accountID, now)
	if err != nil {
		return nil, err
	}
	accountIDs := make([]string, 0, len(touchedThreadIDsByAccount))
	for repairedAccountID := range touchedThreadIDsByAccount {
		accountIDs = append(accountIDs, repairedAccountID)
	}
	for _, repairedAccountID := range accountIDs {
		if err := rebuildThreadIndexOnConn(ctx, conn, repairedAccountID, now); err != nil {
			return nil, err
		}
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return nil, err
	}
	committed = true
	return normalizeThreadIDs(accountID, touchedThreadIDsByAccount[accountID]), nil
}

func repairIndexedThreadHeadersOnConn(ctx context.Context, conn *sql.Conn, accountFilter string, now time.Time) (map[string][]string, error) {
	query := `SELECT account_id,id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,subject,from_value,raw_source
		 FROM message_index`
	args := make([]any, 0, 1)
	if strings.TrimSpace(accountFilter) != "" {
		query += ` WHERE account_id=?`
		args = append(args, strings.TrimSpace(accountFilter))
	}
	query += ` ORDER BY account_id ASC, internal_date ASC, date_header ASC, created_at ASC, id ASC`
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]repairThreadRow, 0, 64)

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
			return nil, err
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

		items = append(items, repairThreadRow{
			accountID:         accountID,
			id:                id,
			subject:           subject,
			fromValue:         fromValue,
			originalThreadID:  mail.NormalizeIndexedThreadID(accountID, threadID),
			threadID:          mail.NormalizeIndexedThreadID(accountID, threadID),
			originalMessageID: mail.NormalizeMessageIDHeader(messageIDHeader),
			messageIDHeader:   normalizedMessageID,
			originalInReplyTo: mail.NormalizeMessageIDHeader(inReplyToHeader),
			inReplyToHeader:   normalizedInReplyTo,
			originalRefs:      mail.FormatMessageIDList(mail.ParseMessageIDList(referencesHeader)),
			referencesHeader:  mail.FormatMessageIDList(normalizedReferences),
			references:        append([]string(nil), normalizedReferences...),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	accountIndexes := map[string]*accountRepairIndex{}
	for i := range items {
		item := &items[i]
		index := accountIndexes[item.accountID]
		if index == nil {
			index = &accountRepairIndex{
				messageRows: map[string]*repairThreadRow{},
				memo:        map[string]string{},
				resolving:   map[string]bool{},
			}
			accountIndexes[item.accountID] = index
		}
		if key := strings.TrimSpace(item.messageIDHeader); key != "" {
			if existing := index.messageRows[key]; shouldReplaceRepairMessageRow(existing, item) {
				index.messageRows[key] = item
			}
		}
	}
	for i := range items {
		item := &items[i]
		index := accountIndexes[item.accountID]
		if index == nil {
			continue
		}
		item.threadID = resolveRepairedIndexedThreadID(index, item)
	}

	touchedAccounts := map[string]struct{}{}
	touchedThreadsByAccount := map[string]map[string]struct{}{}
	addTouchedThread := func(accountID, threadID string) {
		normalized := mail.NormalizeIndexedThreadID(accountID, threadID)
		if normalized == "" {
			return
		}
		set := touchedThreadsByAccount[accountID]
		if set == nil {
			set = map[string]struct{}{}
			touchedThreadsByAccount[accountID] = set
		}
		set[normalized] = struct{}{}
	}
	for _, item := range items {
		if item.originalThreadID == item.threadID &&
			item.originalMessageID == item.messageIDHeader &&
			item.originalInReplyTo == item.inReplyToHeader &&
			item.originalRefs == item.referencesHeader {
			continue
		}
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
			return nil, err
		}
		touchedAccounts[item.accountID] = struct{}{}
		addTouchedThread(item.accountID, item.originalThreadID)
		addTouchedThread(item.accountID, item.threadID)
	}

	accountIDs := make([]string, 0, len(touchedAccounts))
	for accountID := range touchedAccounts {
		accountIDs = append(accountIDs, accountID)
	}
	sort.Strings(accountIDs)
	out := make(map[string][]string, len(accountIDs))
	for _, accountID := range accountIDs {
		set := touchedThreadsByAccount[accountID]
		threads := make([]string, 0, len(set))
		for threadID := range set {
			threads = append(threads, threadID)
		}
		sort.Strings(threads)
		out[accountID] = threads
	}
	return out, nil
}

func shouldReplaceRepairMessageRow(existing, candidate *repairThreadRow) bool {
	if existing == nil {
		return true
	}
	existingScore := len(existing.references)
	if strings.TrimSpace(existing.inReplyToHeader) != "" {
		existingScore++
	}
	candidateScore := len(candidate.references)
	if strings.TrimSpace(candidate.inReplyToHeader) != "" {
		candidateScore++
	}
	if candidateScore != existingScore {
		return candidateScore > existingScore
	}
	return strings.TrimSpace(candidate.id) < strings.TrimSpace(existing.id)
}

func resolveRepairedIndexedThreadID(index *accountRepairIndex, item *repairThreadRow) string {
	if index == nil || item == nil {
		return ""
	}
	if cached := strings.TrimSpace(index.memo[item.id]); cached != "" {
		return cached
	}
	if index.resolving[item.id] {
		return fallbackRepairedThreadID(item)
	}

	index.resolving[item.id] = true
	defer delete(index.resolving, item.id)

	nextThreadID := fallbackRepairedThreadID(item)
	if len(item.references) > 0 {
		rootRef := strings.TrimSpace(item.references[0])
		if rootRef != "" {
			if ancestor := index.messageRows[rootRef]; ancestor != nil && ancestor.id != item.id {
				nextThreadID = resolveRepairedIndexedThreadID(index, ancestor)
			} else if parentID := strings.TrimSpace(item.inReplyToHeader); parentID != "" {
				if parent := index.messageRows[parentID]; parent != nil && parent.id != item.id {
					nextThreadID = resolveRepairedIndexedThreadID(index, parent)
				}
			}
		}
	} else if parentID := strings.TrimSpace(item.inReplyToHeader); parentID != "" {
		if parent := index.messageRows[parentID]; parent != nil && parent.id != item.id {
			nextThreadID = resolveRepairedIndexedThreadID(index, parent)
		}
	}
	index.memo[item.id] = nextThreadID
	return nextThreadID
}

func fallbackRepairedThreadID(item *repairThreadRow) string {
	if item == nil {
		return ""
	}
	return mail.NormalizeIndexedThreadID(item.accountID, mail.DeriveIndexedThreadID(item.messageIDHeader, item.inReplyToHeader, item.references, item.subject, item.fromValue))
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
