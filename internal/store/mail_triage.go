package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"despatch/internal/mail"
	"despatch/internal/models"
)

const (
	mailTriageSourceLive    = "live"
	mailTriageSourceIndexed = "indexed"
)

func normalizeMailTriageSource(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case mailTriageSourceIndexed:
		return mailTriageSourceIndexed
	default:
		return mailTriageSourceLive
	}
}

func normalizeMailTriageName(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func normalizeMailTriageTarget(in models.MailTriageTarget) models.MailTriageTarget {
	source := normalizeMailTriageSource(in.Source)
	accountID := strings.TrimSpace(in.AccountID)
	threadID := strings.TrimSpace(in.ThreadID)
	if source == mailTriageSourceIndexed {
		accountID = strings.TrimSpace(accountID)
		threadID = mail.NormalizeIndexedThreadID(accountID, threadID)
	}
	return models.MailTriageTarget{
		Source:    source,
		AccountID: accountID,
		ThreadID:  threadID,
		Mailbox:   strings.TrimSpace(in.Mailbox),
		Subject:   strings.TrimSpace(in.Subject),
		From:      strings.TrimSpace(in.From),
	}
}

func normalizeMailTriageTargets(items []models.MailTriageTarget) []models.MailTriageTarget {
	seen := map[string]struct{}{}
	out := make([]models.MailTriageTarget, 0, len(items))
	for _, item := range items {
		normalized := normalizeMailTriageTarget(item)
		if normalized.ThreadID == "" {
			continue
		}
		key := normalized.Source + "\x00" + normalized.AccountID + "\x00" + normalized.ThreadID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func deriveMailThreadTriageKey(userID string, target models.MailTriageTarget) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(userID),
		strings.TrimSpace(target.Source),
		strings.TrimSpace(target.AccountID),
		strings.TrimSpace(target.ThreadID),
	}, "\n")))
	return "tri:" + hex.EncodeToString(sum[:12])
}

func scanMailTriageCategory(scanner interface{ Scan(dest ...any) error }) (models.MailTriageCategory, error) {
	var item models.MailTriageCategory
	if err := scanner.Scan(&item.ID, &item.UserID, &item.Name, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return models.MailTriageCategory{}, err
	}
	item.Name = normalizeMailTriageName(item.Name)
	return item, nil
}

func scanMailTriageTag(scanner interface{ Scan(dest ...any) error }) (models.MailTriageTag, error) {
	var item models.MailTriageTag
	if err := scanner.Scan(&item.ID, &item.UserID, &item.Name, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return models.MailTriageTag{}, err
	}
	item.Name = normalizeMailTriageName(item.Name)
	return item, nil
}

func (s *Store) ListMailTriageCategories(ctx context.Context, userID string) ([]models.MailTriageCategory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,name,created_at,updated_at
		 FROM mail_triage_categories
		 WHERE user_id=?
		 ORDER BY name COLLATE NOCASE ASC, id ASC`,
		userID,
	)
	if err != nil {
		if isOptionalSchemaErr(err) {
			return []models.MailTriageCategory{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]models.MailTriageCategory, 0, 16)
	for rows.Next() {
		item, scanErr := scanMailTriageCategory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListMailTriageTags(ctx context.Context, userID string) ([]models.MailTriageTag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,name,created_at,updated_at
		 FROM mail_triage_tags
		 WHERE user_id=?
		 ORDER BY name COLLATE NOCASE ASC, id ASC`,
		userID,
	)
	if err != nil {
		if isOptionalSchemaErr(err) {
			return []models.MailTriageTag{}, nil
		}
		return nil, err
	}
	defer rows.Close()
	out := make([]models.MailTriageTag, 0, 24)
	for rows.Next() {
		item, scanErr := scanMailTriageTag(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateMailTriageCategory(ctx context.Context, in models.MailTriageCategory) (models.MailTriageCategory, error) {
	name := normalizeMailTriageName(in.Name)
	if name == "" {
		return models.MailTriageCategory{}, fmt.Errorf("category name is required")
	}
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.Name = name
	in.CreatedAt = now
	in.UpdatedAt = now
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO mail_triage_categories(id,user_id,name,name_norm,created_at,updated_at)
		 VALUES(?,?,?,?,?,?)`,
		in.ID, in.UserID, in.Name, strings.ToLower(in.Name), in.CreatedAt, in.UpdatedAt,
	); err != nil {
		if isOptionalSchemaErr(err) {
			return models.MailTriageCategory{}, ErrNotFound
		}
		if isUniqueConstraintErr(err) {
			return models.MailTriageCategory{}, ErrConflict
		}
		return models.MailTriageCategory{}, err
	}
	return in, nil
}

func (s *Store) UpdateMailTriageCategory(ctx context.Context, in models.MailTriageCategory) (models.MailTriageCategory, error) {
	name := normalizeMailTriageName(in.Name)
	if name == "" {
		return models.MailTriageCategory{}, fmt.Errorf("category name is required")
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE mail_triage_categories
		 SET name=?, name_norm=?, updated_at=?
		 WHERE user_id=? AND id=?`,
		name, strings.ToLower(name), now, in.UserID, in.ID,
	)
	if err != nil {
		if isOptionalSchemaErr(err) {
			return models.MailTriageCategory{}, ErrNotFound
		}
		if isUniqueConstraintErr(err) {
			return models.MailTriageCategory{}, ErrConflict
		}
		return models.MailTriageCategory{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.MailTriageCategory{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,name,created_at,updated_at
		 FROM mail_triage_categories
		 WHERE user_id=? AND id=?`,
		in.UserID, in.ID,
	)
	return scanMailTriageCategory(row)
}

func (s *Store) DeleteMailTriageCategory(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mail_triage_categories WHERE user_id=? AND id=?`, userID, id)
	if err != nil {
		if isOptionalSchemaErr(err) {
			return ErrNotFound
		}
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateMailTriageTag(ctx context.Context, in models.MailTriageTag) (models.MailTriageTag, error) {
	name := normalizeMailTriageName(in.Name)
	if name == "" {
		return models.MailTriageTag{}, fmt.Errorf("tag name is required")
	}
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.Name = name
	in.CreatedAt = now
	in.UpdatedAt = now
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO mail_triage_tags(id,user_id,name,name_norm,created_at,updated_at)
		 VALUES(?,?,?,?,?,?)`,
		in.ID, in.UserID, in.Name, strings.ToLower(in.Name), in.CreatedAt, in.UpdatedAt,
	); err != nil {
		if isOptionalSchemaErr(err) {
			return models.MailTriageTag{}, ErrNotFound
		}
		if isUniqueConstraintErr(err) {
			return models.MailTriageTag{}, ErrConflict
		}
		return models.MailTriageTag{}, err
	}
	return in, nil
}

func (s *Store) UpdateMailTriageTag(ctx context.Context, in models.MailTriageTag) (models.MailTriageTag, error) {
	name := normalizeMailTriageName(in.Name)
	if name == "" {
		return models.MailTriageTag{}, fmt.Errorf("tag name is required")
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE mail_triage_tags
		 SET name=?, name_norm=?, updated_at=?
		 WHERE user_id=? AND id=?`,
		name, strings.ToLower(name), now, in.UserID, in.ID,
	)
	if err != nil {
		if isOptionalSchemaErr(err) {
			return models.MailTriageTag{}, ErrNotFound
		}
		if isUniqueConstraintErr(err) {
			return models.MailTriageTag{}, ErrConflict
		}
		return models.MailTriageTag{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.MailTriageTag{}, ErrNotFound
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,name,created_at,updated_at
		 FROM mail_triage_tags
		 WHERE user_id=? AND id=?`,
		in.UserID, in.ID,
	)
	return scanMailTriageTag(row)
}

func (s *Store) DeleteMailTriageTag(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mail_triage_tags WHERE user_id=? AND id=?`, userID, id)
	if err != nil {
		if isOptionalSchemaErr(err) {
			return ErrNotFound
		}
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func buildMailThreadTriageState(now time.Time, target models.MailTriageTarget, record models.MailThreadTriageRecord, category *models.MailTriageCategoryRef, tags []models.MailTriageTagRef) models.MailThreadTriageState {
	state := models.DefaultMailTriageState()
	if !record.SnoozedUntil.IsZero() {
		value := record.SnoozedUntil.UTC()
		state.SnoozedUntil = &value
		state.IsSnoozed = value.After(now)
	}
	if !record.ReminderAt.IsZero() {
		value := record.ReminderAt.UTC()
		state.ReminderAt = &value
		state.IsFollowUpDue = !value.After(now)
	}
	state.Category = category
	state.Tags = append([]models.MailTriageTagRef{}, tags...)
	return models.MailThreadTriageState{
		Target:    target,
		TriageKey: deriveMailThreadTriageKey(record.UserID, target),
		Triage:    state,
	}
}

func buildDefaultMailThreadTriageState(now time.Time, userID string, target models.MailTriageTarget) models.MailThreadTriageState {
	return buildMailThreadTriageState(now, target, models.MailThreadTriageRecord{
		UserID:    userID,
		Source:    target.Source,
		AccountID: target.AccountID,
		ThreadID:  target.ThreadID,
	}, nil, nil)
}

func (s *Store) GetMailThreadTriageStates(ctx context.Context, userID string, targets []models.MailTriageTarget) ([]models.MailThreadTriageState, error) {
	normalizedTargets := normalizeMailTriageTargets(targets)
	if len(normalizedTargets) == 0 {
		return []models.MailThreadTriageState{}, nil
	}
	now := time.Now().UTC()
	keys := make([]string, 0, len(normalizedTargets))
	targetByKey := make(map[string]models.MailTriageTarget, len(normalizedTargets))
	for _, target := range normalizedTargets {
		key := deriveMailThreadTriageKey(userID, target)
		keys = append(keys, key)
		targetByKey[key] = target
	}

	queryArgs := make([]any, 0, len(keys)+1)
	queryArgs = append(queryArgs, userID)
	for _, key := range keys {
		queryArgs = append(queryArgs, key)
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT triage_key,user_id,source,account_id,thread_id,mailbox_name,subject,from_value,category_id,snoozed_until,reminder_at,last_reminder_notified_at,created_at,updated_at
		 FROM mail_thread_triage
		 WHERE user_id=? AND triage_key IN (%s)`, placeholders(len(keys))),
		queryArgs...,
	)
	if err != nil {
		if isOptionalSchemaErr(err) {
			out := make([]models.MailThreadTriageState, 0, len(normalizedTargets))
			for _, target := range normalizedTargets {
				out = append(out, buildDefaultMailThreadTriageState(now, userID, target))
			}
			return out, nil
		}
		return nil, err
	}
	defer rows.Close()

	recordsByKey := make(map[string]models.MailThreadTriageRecord, len(keys))
	categoryIDs := map[string]struct{}{}
	for rows.Next() {
		var (
			record                  models.MailThreadTriageRecord
			categoryID              sql.NullString
			snoozedUntil            sql.NullTime
			reminderAt              sql.NullTime
			lastReminderNotifiedAt  sql.NullTime
		)
		if err := rows.Scan(
			&record.TriageKey,
			&record.UserID,
			&record.Source,
			&record.AccountID,
			&record.ThreadID,
			&record.Mailbox,
			&record.Subject,
			&record.FromValue,
			&categoryID,
			&snoozedUntil,
			&reminderAt,
			&lastReminderNotifiedAt,
			&record.CreatedAt,
			&record.UpdatedAt,
		); err != nil {
			return nil, err
		}
		record.Source = normalizeMailTriageSource(record.Source)
		record.AccountID = strings.TrimSpace(record.AccountID)
		record.ThreadID = strings.TrimSpace(record.ThreadID)
		record.Mailbox = strings.TrimSpace(record.Mailbox)
		record.Subject = strings.TrimSpace(record.Subject)
		record.FromValue = strings.TrimSpace(record.FromValue)
		record.CategoryID = strings.TrimSpace(categoryID.String)
		if snoozedUntil.Valid {
			record.SnoozedUntil = snoozedUntil.Time.UTC()
		}
		if reminderAt.Valid {
			record.ReminderAt = reminderAt.Time.UTC()
		}
		if lastReminderNotifiedAt.Valid {
			record.LastReminderNotifiedAt = lastReminderNotifiedAt.Time.UTC()
		}
		if record.CategoryID != "" {
			categoryIDs[record.CategoryID] = struct{}{}
		}
		recordsByKey[record.TriageKey] = record
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	categoriesByID := map[string]models.MailTriageCategoryRef{}
	if len(categoryIDs) > 0 {
		args := make([]any, 0, len(categoryIDs)+1)
		args = append(args, userID)
		ids := make([]string, 0, len(categoryIDs))
		for id := range categoryIDs {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			args = append(args, id)
		}
		categoryRows, categoryErr := s.db.QueryContext(ctx,
			fmt.Sprintf(`SELECT id,name FROM mail_triage_categories WHERE user_id=? AND id IN (%s)`, placeholders(len(ids))),
			args...,
		)
		if categoryErr != nil {
			if !isOptionalSchemaErr(categoryErr) {
				return nil, categoryErr
			}
		} else {
			for categoryRows.Next() {
				var item models.MailTriageCategoryRef
				if err := categoryRows.Scan(&item.ID, &item.Name); err != nil {
					_ = categoryRows.Close()
					return nil, err
				}
				item.Name = normalizeMailTriageName(item.Name)
				categoriesByID[item.ID] = item
			}
			if err := categoryRows.Err(); err != nil {
				_ = categoryRows.Close()
				return nil, err
			}
			_ = categoryRows.Close()
		}
	}

	tagsByKey := map[string][]models.MailTriageTagRef{}
	tagRows, tagErr := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT tt.triage_key,t.id,t.name
		 FROM mail_thread_triage_tags tt
		 JOIN mail_triage_tags t ON t.id=tt.tag_id
		 WHERE tt.triage_key IN (%s)
		 ORDER BY t.name COLLATE NOCASE ASC, t.id ASC`, placeholders(len(keys))),
		stringSliceToAny(keys)...,
	)
	if tagErr != nil {
		if !isOptionalSchemaErr(tagErr) {
			return nil, tagErr
		}
	} else {
		defer tagRows.Close()
		for tagRows.Next() {
			var (
				triageKey string
				tag       models.MailTriageTagRef
			)
			if err := tagRows.Scan(&triageKey, &tag.ID, &tag.Name); err != nil {
				return nil, err
			}
			tag.Name = normalizeMailTriageName(tag.Name)
			tagsByKey[triageKey] = append(tagsByKey[triageKey], tag)
		}
		if err := tagRows.Err(); err != nil {
			return nil, err
		}
	}

	out := make([]models.MailThreadTriageState, 0, len(normalizedTargets))
	for _, target := range normalizedTargets {
		key := deriveMailThreadTriageKey(userID, target)
		record, ok := recordsByKey[key]
		if !ok {
			out = append(out, buildDefaultMailThreadTriageState(now, userID, target))
			continue
		}
		var category *models.MailTriageCategoryRef
		if item, ok := categoriesByID[record.CategoryID]; ok {
			copyItem := item
			category = &copyItem
		}
		out = append(out, buildMailThreadTriageState(now, targetByKey[key], record, category, tagsByKey[key]))
	}
	return out, nil
}

func stringSliceToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func getMailTriageCategoryTx(ctx context.Context, tx *sql.Tx, userID, id string) (models.MailTriageCategory, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT id,user_id,name,created_at,updated_at
		 FROM mail_triage_categories
		 WHERE user_id=? AND id=?`,
		userID, id,
	)
	item, err := scanMailTriageCategory(row)
	if err == sql.ErrNoRows {
		return models.MailTriageCategory{}, ErrNotFound
	}
	return item, err
}

func resolveMailTriageCategoryIDTx(ctx context.Context, tx *sql.Tx, userID, categoryID, categoryName string) (string, error) {
	if trimmed := strings.TrimSpace(categoryID); trimmed != "" {
		item, err := getMailTriageCategoryTx(ctx, tx, userID, trimmed)
		if err != nil {
			return "", err
		}
		return item.ID, nil
	}
	name := normalizeMailTriageName(categoryName)
	if name == "" {
		return "", nil
	}
	row := tx.QueryRowContext(ctx,
		`SELECT id,user_id,name,created_at,updated_at
		 FROM mail_triage_categories
		 WHERE user_id=? AND name_norm=?`,
		userID, strings.ToLower(name),
	)
	item, err := scanMailTriageCategory(row)
	if err == nil {
		return item.ID, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	now := time.Now().UTC()
	id := uuid.NewString()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO mail_triage_categories(id,user_id,name,name_norm,created_at,updated_at)
		 VALUES(?,?,?,?,?,?)`,
		id, userID, name, strings.ToLower(name), now, now,
	); err != nil {
		if isUniqueConstraintErr(err) {
			return resolveMailTriageCategoryIDTx(ctx, tx, userID, "", name)
		}
		return "", err
	}
	return id, nil
}

func resolveMailTriageTagIDsTx(ctx context.Context, tx *sql.Tx, userID string, ids, names []string, createMissing bool) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids)+len(names))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		item, err := getMailTriageTagTx(ctx, tx, userID, trimmed)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		out = append(out, item.ID)
	}
	for _, rawName := range names {
		name := normalizeMailTriageName(rawName)
		if name == "" {
			continue
		}
		item, err := getMailTriageTagByNameTx(ctx, tx, userID, name)
		if err == nil {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			out = append(out, item.ID)
			continue
		}
		if err != ErrNotFound {
			return nil, err
		}
		if !createMissing {
			continue
		}
		now := time.Now().UTC()
		item = models.MailTriageTag{
			ID:        uuid.NewString(),
			UserID:    userID,
			Name:      name,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO mail_triage_tags(id,user_id,name,name_norm,created_at,updated_at)
			 VALUES(?,?,?,?,?,?)`,
			item.ID, item.UserID, item.Name, strings.ToLower(item.Name), item.CreatedAt, item.UpdatedAt,
		); err != nil {
			if isUniqueConstraintErr(err) {
				item, err = getMailTriageTagByNameTx(ctx, tx, userID, name)
				if err != nil {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		out = append(out, item.ID)
	}
	return out, nil
}

func getMailTriageTagTx(ctx context.Context, tx *sql.Tx, userID, id string) (models.MailTriageTag, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT id,user_id,name,created_at,updated_at
		 FROM mail_triage_tags
		 WHERE user_id=? AND id=?`,
		userID, id,
	)
	item, err := scanMailTriageTag(row)
	if err == sql.ErrNoRows {
		return models.MailTriageTag{}, ErrNotFound
	}
	return item, err
}

func getMailTriageTagByNameTx(ctx context.Context, tx *sql.Tx, userID, name string) (models.MailTriageTag, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT id,user_id,name,created_at,updated_at
		 FROM mail_triage_tags
		 WHERE user_id=? AND name_norm=?`,
		userID, strings.ToLower(normalizeMailTriageName(name)),
	)
	item, err := scanMailTriageTag(row)
	if err == sql.ErrNoRows {
		return models.MailTriageTag{}, ErrNotFound
	}
	return item, err
}

func getMailThreadTriageRecordTx(ctx context.Context, tx *sql.Tx, userID string, target models.MailTriageTarget) (models.MailThreadTriageRecord, error) {
	key := deriveMailThreadTriageKey(userID, target)
	row := tx.QueryRowContext(ctx,
		`SELECT triage_key,user_id,source,account_id,thread_id,mailbox_name,subject,from_value,category_id,snoozed_until,reminder_at,last_reminder_notified_at,created_at,updated_at
		 FROM mail_thread_triage
		 WHERE triage_key=?`,
		key,
	)
	var (
		record                 models.MailThreadTriageRecord
		categoryID             sql.NullString
		snoozedUntil           sql.NullTime
		reminderAt             sql.NullTime
		lastReminderNotifiedAt sql.NullTime
	)
	if err := row.Scan(
		&record.TriageKey,
		&record.UserID,
		&record.Source,
		&record.AccountID,
		&record.ThreadID,
		&record.Mailbox,
		&record.Subject,
		&record.FromValue,
		&categoryID,
		&snoozedUntil,
		&reminderAt,
		&lastReminderNotifiedAt,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return models.MailThreadTriageRecord{}, ErrNotFound
		}
		return models.MailThreadTriageRecord{}, err
	}
	record.Source = normalizeMailTriageSource(record.Source)
	record.AccountID = strings.TrimSpace(record.AccountID)
	record.ThreadID = strings.TrimSpace(record.ThreadID)
	record.Mailbox = strings.TrimSpace(record.Mailbox)
	record.Subject = strings.TrimSpace(record.Subject)
	record.FromValue = strings.TrimSpace(record.FromValue)
	record.CategoryID = strings.TrimSpace(categoryID.String)
	if snoozedUntil.Valid {
		record.SnoozedUntil = snoozedUntil.Time.UTC()
	}
	if reminderAt.Valid {
		record.ReminderAt = reminderAt.Time.UTC()
	}
	if lastReminderNotifiedAt.Valid {
		record.LastReminderNotifiedAt = lastReminderNotifiedAt.Time.UTC()
	}
	return record, nil
}

func countMailThreadTriageTagsTx(ctx context.Context, tx *sql.Tx, triageKey string) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM mail_thread_triage_tags WHERE triage_key=?`, triageKey).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func mailThreadTriageHasState(record models.MailThreadTriageRecord, tagCount int) bool {
	return !record.SnoozedUntil.IsZero() || !record.ReminderAt.IsZero() || strings.TrimSpace(record.CategoryID) != "" || tagCount > 0
}

func (s *Store) ApplyMailThreadTriage(ctx context.Context, userID string, targets []models.MailTriageTarget, mutation models.MailTriageMutation) ([]models.MailThreadTriageState, error) {
	normalizedTargets := normalizeMailTriageTargets(targets)
	if len(normalizedTargets) == 0 {
		return []models.MailThreadTriageState{}, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	categoryID := ""
	if mutation.ClearCategory {
		categoryID = ""
	} else {
		categoryID, err = resolveMailTriageCategoryIDTx(ctx, tx, userID, mutation.CategoryID, mutation.CategoryName)
		if err != nil && err != ErrNotFound {
			return nil, err
		}
	}
	addTagIDs, err := resolveMailTriageTagIDsTx(ctx, tx, userID, mutation.AddTagIDs, mutation.AddTagNames, true)
	if err != nil {
		return nil, err
	}
	removeTagIDs, err := resolveMailTriageTagIDsTx(ctx, tx, userID, mutation.RemoveTagIDs, mutation.RemoveTagNames, false)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	for _, target := range normalizedTargets {
		record, getErr := getMailThreadTriageRecordTx(ctx, tx, userID, target)
		if getErr != nil && getErr != ErrNotFound {
			return nil, getErr
		}
		hasMutationPayload := mutation.SnoozedUntil != nil ||
			mutation.ClearSnooze ||
			mutation.ReminderAt != nil ||
			mutation.ClearReminder ||
			categoryID != "" ||
			mutation.ClearCategory ||
			len(addTagIDs) > 0 ||
			len(removeTagIDs) > 0 ||
			mutation.ClearTags
		createRecord := getErr == ErrNotFound && hasMutationPayload
		if createRecord {
			record = models.MailThreadTriageRecord{
				TriageKey: deriveMailThreadTriageKey(userID, target),
				UserID:    userID,
				Source:    target.Source,
				AccountID: target.AccountID,
				ThreadID:  target.ThreadID,
				Mailbox:   target.Mailbox,
				Subject:   target.Subject,
				FromValue: target.From,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO mail_thread_triage(
					triage_key,user_id,source,account_id,thread_id,mailbox_name,subject,from_value,category_id,snoozed_until,reminder_at,last_reminder_notified_at,created_at,updated_at
				) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
				record.TriageKey,
				record.UserID,
				record.Source,
				record.AccountID,
				record.ThreadID,
				record.Mailbox,
				record.Subject,
				record.FromValue,
				nil,
				nil,
				nil,
				nil,
				record.CreatedAt,
				record.UpdatedAt,
			); err != nil {
				return nil, err
			}
		} else if getErr == ErrNotFound {
			continue
		}

		if trimmed := strings.TrimSpace(target.Mailbox); trimmed != "" {
			record.Mailbox = trimmed
		}
		if trimmed := strings.TrimSpace(target.Subject); trimmed != "" {
			record.Subject = trimmed
		}
		if trimmed := strings.TrimSpace(target.From); trimmed != "" {
			record.FromValue = trimmed
		}
		if mutation.SnoozedUntil != nil {
			record.SnoozedUntil = mutation.SnoozedUntil.UTC()
		}
		if mutation.ClearSnooze {
			record.SnoozedUntil = time.Time{}
		}
		if mutation.ReminderAt != nil {
			record.ReminderAt = mutation.ReminderAt.UTC()
			record.LastReminderNotifiedAt = time.Time{}
		}
		if mutation.ClearReminder {
			record.ReminderAt = time.Time{}
			record.LastReminderNotifiedAt = time.Time{}
		}
		switch {
		case mutation.ClearCategory:
			record.CategoryID = ""
		case categoryID != "":
			record.CategoryID = categoryID
		}
		record.UpdatedAt = now

		if _, err := tx.ExecContext(ctx,
			`UPDATE mail_thread_triage
			 SET mailbox_name=?, subject=?, from_value=?, category_id=?, snoozed_until=?, reminder_at=?, last_reminder_notified_at=?, updated_at=?
			 WHERE triage_key=?`,
			record.Mailbox,
			record.Subject,
			record.FromValue,
			nullStringValue(record.CategoryID),
			nullTimeValue(record.SnoozedUntil),
			nullTimeValue(record.ReminderAt),
			nullTimeValue(record.LastReminderNotifiedAt),
			record.UpdatedAt,
			record.TriageKey,
		); err != nil {
			return nil, err
		}

		if mutation.ClearTags {
			if _, err := tx.ExecContext(ctx, `DELETE FROM mail_thread_triage_tags WHERE triage_key=?`, record.TriageKey); err != nil {
				return nil, err
			}
		}
		for _, tagID := range addTagIDs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO mail_thread_triage_tags(triage_key,tag_id,created_at)
				 VALUES(?,?,?)
				 ON CONFLICT(triage_key,tag_id) DO NOTHING`,
				record.TriageKey, tagID, now,
			); err != nil {
				return nil, err
			}
		}
		if len(removeTagIDs) > 0 {
			args := make([]any, 0, len(removeTagIDs)+1)
			args = append(args, record.TriageKey)
			for _, tagID := range removeTagIDs {
				args = append(args, tagID)
			}
			if _, err := tx.ExecContext(ctx,
				fmt.Sprintf(`DELETE FROM mail_thread_triage_tags WHERE triage_key=? AND tag_id IN (%s)`, placeholders(len(removeTagIDs))),
				args...,
			); err != nil {
				return nil, err
			}
		}

		tagCount, err := countMailThreadTriageTagsTx(ctx, tx, record.TriageKey)
		if err != nil {
			return nil, err
		}
		if !mailThreadTriageHasState(record, tagCount) {
			if _, err := tx.ExecContext(ctx, `DELETE FROM mail_thread_triage WHERE triage_key=?`, record.TriageKey); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetMailThreadTriageStates(ctx, userID, normalizedTargets)
}

func (s *Store) PollDueMailTriageReminders(ctx context.Context, userID string, limit int) ([]models.MailTriageReminder, error) {
	if limit <= 0 {
		limit = 24
	}
	if limit > 100 {
		limit = 100
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	rows, err := tx.QueryContext(ctx,
		`SELECT triage_key,source,account_id,thread_id,mailbox_name,subject,from_value,reminder_at
		 FROM mail_thread_triage
		 WHERE user_id=?
		   AND reminder_at IS NOT NULL
		   AND reminder_at<=?
		   AND (last_reminder_notified_at IS NULL OR last_reminder_notified_at < reminder_at)
		 ORDER BY reminder_at ASC, updated_at DESC
		 LIMIT ?`,
		userID, now, limit,
	)
	if err != nil {
		if isOptionalSchemaErr(err) {
			return []models.MailTriageReminder{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	items := make([]models.MailTriageReminder, 0, limit)
	keys := make([]string, 0, limit)
	updates := make(map[string]time.Time, limit)
	for rows.Next() {
		var (
			item       models.MailTriageReminder
			reminderAt sql.NullTime
		)
		if err := rows.Scan(&item.TriageKey, &item.Source, &item.AccountID, &item.ThreadID, &item.Mailbox, &item.Subject, &item.From, &reminderAt); err != nil {
			return nil, err
		}
		if !reminderAt.Valid {
			continue
		}
		item.Source = normalizeMailTriageSource(item.Source)
		item.AccountID = strings.TrimSpace(item.AccountID)
		item.ThreadID = strings.TrimSpace(item.ThreadID)
		item.Mailbox = strings.TrimSpace(item.Mailbox)
		item.Subject = strings.TrimSpace(item.Subject)
		item.From = strings.TrimSpace(item.From)
		item.ReminderAt = reminderAt.Time.UTC()
		items = append(items, item)
		keys = append(keys, item.TriageKey)
		updates[item.TriageKey] = item.ReminderAt
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []models.MailTriageReminder{}, nil
	}
	for _, item := range items {
		if _, err := tx.ExecContext(ctx,
			`UPDATE mail_thread_triage
			 SET last_reminder_notified_at=?, updated_at=?
			 WHERE triage_key=?`,
			updates[item.TriageKey], now, item.TriageKey,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}
