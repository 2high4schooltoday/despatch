package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"despatch/internal/mail"
)

type SyncState struct {
	ID              string
	AccountID       string
	Mailbox         string
	UIDValidity     uint32
	UIDNext         uint32
	ModSeq          uint64
	LastFullSyncAt  time.Time
	LastDeltaSyncAt time.Time
	LastError       string
	UpdatedAt       time.Time
}

func (s *Store) GetSyncState(ctx context.Context, accountID, mailbox string) (SyncState, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,account_id,mailbox,uid_validity,uid_next,mod_seq,last_full_sync_at,last_delta_sync_at,last_error,updated_at
		 FROM sync_state
		 WHERE account_id=? AND mailbox=?`,
		accountID,
		strings.TrimSpace(mailbox),
	)
	return scanSyncState(row)
}

func (s *Store) UpsertSyncState(ctx context.Context, in SyncState) (SyncState, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.AccountID = strings.TrimSpace(in.AccountID)
	in.Mailbox = strings.TrimSpace(in.Mailbox)
	in.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sync_state(
			id,account_id,mailbox,uid_validity,uid_next,mod_seq,last_full_sync_at,last_delta_sync_at,last_error,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(account_id, mailbox) DO UPDATE SET
			uid_validity=excluded.uid_validity,
			uid_next=excluded.uid_next,
			mod_seq=excluded.mod_seq,
			last_full_sync_at=excluded.last_full_sync_at,
			last_delta_sync_at=excluded.last_delta_sync_at,
			last_error=excluded.last_error,
			updated_at=excluded.updated_at`,
		in.ID,
		in.AccountID,
		in.Mailbox,
		in.UIDValidity,
		in.UIDNext,
		in.ModSeq,
		nullTimeValue(in.LastFullSyncAt),
		nullTimeValue(in.LastDeltaSyncAt),
		nullStringValue(in.LastError),
		in.UpdatedAt,
	)
	if err != nil {
		return SyncState{}, err
	}
	return s.GetSyncState(ctx, in.AccountID, in.Mailbox)
}

func (s *Store) DeleteIndexedMessagesByMailbox(ctx context.Context, accountID, mailbox string) error {
	mailbox = strings.TrimSpace(mailbox)
	if mailbox == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM attachment_index
		 WHERE account_id=? AND message_id IN (
		   SELECT id FROM message_index WHERE account_id=? AND mailbox=?
		 )`,
		accountID,
		accountID,
		mailbox,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM message_index WHERE account_id=? AND mailbox=?`, accountID, mailbox); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteIndexedDataByAccount(ctx context.Context, accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM attachment_index WHERE account_id=?`, accountID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM message_index WHERE account_id=?`, accountID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM thread_index WHERE account_id=?`, accountID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sync_state WHERE account_id=?`, accountID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) listMailboxThreadIDs(ctx context.Context, accountID string, mailboxes []string) ([]string, error) {
	normalizedMailboxes := make([]string, 0, len(mailboxes))
	seen := map[string]struct{}{}
	for _, mailbox := range mailboxes {
		trimmed := strings.TrimSpace(mailbox)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalizedMailboxes = append(normalizedMailboxes, trimmed)
	}
	if len(normalizedMailboxes) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(normalizedMailboxes)+1)
	args = append(args, accountID)
	for _, mailbox := range normalizedMailboxes {
		args = append(args, mailbox)
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT DISTINCT thread_id FROM message_index WHERE account_id=? AND mailbox IN (%s)`, placeholders(len(normalizedMailboxes))),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	threadIDs := make([]string, 0, len(normalizedMailboxes))
	for rows.Next() {
		var threadID string
		if err := rows.Scan(&threadID); err != nil {
			return nil, err
		}
		threadIDs = append(threadIDs, threadID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return normalizeThreadIDs(accountID, threadIDs), nil
}

func (s *Store) RenameIndexedMailbox(ctx context.Context, accountID, mailbox, newMailbox string) ([]string, error) {
	mailbox = strings.TrimSpace(mailbox)
	newMailbox = strings.TrimSpace(newMailbox)
	if mailbox == "" || newMailbox == "" || strings.EqualFold(mailbox, newMailbox) {
		return nil, nil
	}
	threadIDs, err := s.listMailboxThreadIDs(ctx, accountID, []string{mailbox})
	if err != nil {
		return nil, err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE message_index SET mailbox=?, updated_at=? WHERE account_id=? AND mailbox=?`,
		newMailbox,
		time.Now().UTC(),
		accountID,
		mailbox,
	)
	if err != nil {
		return nil, err
	}
	return threadIDs, nil
}

func (s *Store) DeleteIndexedMailbox(ctx context.Context, accountID, mailbox string) ([]string, error) {
	mailbox = strings.TrimSpace(mailbox)
	if mailbox == "" {
		return nil, nil
	}
	threadIDs, err := s.listMailboxThreadIDs(ctx, accountID, []string{mailbox})
	if err != nil {
		return nil, err
	}
	if err := s.DeleteIndexedMessagesByMailbox(ctx, accountID, mailbox); err != nil {
		return nil, err
	}
	return threadIDs, nil
}

func (s *Store) DeleteSyncState(ctx context.Context, accountID, mailbox string) error {
	mailbox = strings.TrimSpace(mailbox)
	if mailbox == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM sync_state WHERE account_id=? AND mailbox=?`, accountID, mailbox)
	return err
}

func (s *Store) RefreshThreadIndex(ctx context.Context, accountID string, threadIDs []string) error {
	normalized := normalizeThreadIDs(accountID, threadIDs)
	if len(normalized) == 0 {
		return nil
	}

	queryArgs := make([]any, 0, len(normalized)+1)
	queryArgs = append(queryArgs, accountID)
	for _, threadID := range normalized {
		queryArgs = append(queryArgs, threadID)
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(
			`SELECT id,mailbox,thread_id,subject,from_value,seen,has_attachments,flagged,importance,internal_date
			 FROM message_index
			 WHERE account_id=? AND thread_id IN (%s)
			 ORDER BY internal_date DESC`,
			placeholders(len(normalized)),
		),
		queryArgs...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type threadAgg struct {
		ID             string
		AccountID      string
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
	aggs := make(map[string]*threadAgg, len(normalized))
	for rows.Next() {
		var (
			messageID    string
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
		if err := rows.Scan(&messageID, &mailbox, &threadID, &subject, &fromValue, &seenInt, &attachInt, &flaggedInt, &importance, &internalDate); err != nil {
			return err
		}
		scopedMessageID := mail.NormalizeIndexedMessageID(accountID, messageID)
		scopedThreadID := mail.NormalizeIndexedThreadID(accountID, threadID)
		agg := aggs[scopedThreadID]
		if agg == nil {
			agg = &threadAgg{
				ID:           scopedThreadID,
				AccountID:    accountID,
				Mailbox:      mailbox,
				SubjectNorm:  threadSummarySubject(subject),
				Participants: map[string]struct{}{},
			}
			aggs[scopedThreadID] = agg
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	for _, agg := range aggs {
		participants := make([]string, 0, len(agg.Participants))
		for participant := range agg.Participants {
			participants = append(participants, participant)
		}
		sort.Strings(participants)
		participantsJSON, _ := json.Marshal(participants)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO thread_index(
				id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
				account_id=excluded.account_id,
				mailbox=excluded.mailbox,
				subject_norm=excluded.subject_norm,
				participants_json=excluded.participants_json,
				message_count=excluded.message_count,
				unread_count=excluded.unread_count,
				has_attachments=excluded.has_attachments,
				has_flagged=excluded.has_flagged,
				importance=excluded.importance,
				latest_message_id=excluded.latest_message_id,
				latest_at=excluded.latest_at,
				updated_at=excluded.updated_at`,
			agg.ID,
			agg.AccountID,
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
	missing := make([]string, 0, len(normalized))
	for _, threadID := range normalized {
		if _, ok := aggs[threadID]; ok {
			continue
		}
		missing = append(missing, threadID)
	}
	if len(missing) > 0 {
		deleteArgs := make([]any, 0, len(missing)+1)
		deleteArgs = append(deleteArgs, accountID)
		for _, threadID := range missing {
			deleteArgs = append(deleteArgs, threadID)
		}
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM thread_index WHERE account_id=? AND id IN (%s)`, placeholders(len(missing))),
			deleteArgs...,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanSyncState(scanner interface{ Scan(dest ...any) error }) (SyncState, error) {
	var item SyncState
	var lastFullSyncAt sql.NullTime
	var lastDeltaSyncAt sql.NullTime
	var lastError sql.NullString
	err := scanner.Scan(
		&item.ID,
		&item.AccountID,
		&item.Mailbox,
		&item.UIDValidity,
		&item.UIDNext,
		&item.ModSeq,
		&lastFullSyncAt,
		&lastDeltaSyncAt,
		&lastError,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return SyncState{}, ErrNotFound
	}
	if err != nil {
		return SyncState{}, err
	}
	if lastFullSyncAt.Valid {
		item.LastFullSyncAt = lastFullSyncAt.Time
	}
	if lastDeltaSyncAt.Valid {
		item.LastDeltaSyncAt = lastDeltaSyncAt.Time
	}
	if lastError.Valid {
		item.LastError = lastError.String
	}
	return item, nil
}

func normalizeThreadIDs(accountID string, threadIDs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(threadIDs))
	for _, threadID := range threadIDs {
		normalized := mail.NormalizeIndexedThreadID(accountID, threadID)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	items := make([]string, n)
	for i := range items {
		items[i] = "?"
	}
	return strings.Join(items, ",")
}
