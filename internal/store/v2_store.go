package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	stdmail "net/mail"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"despatch/internal/mail"
	"despatch/internal/models"
)

const indexedMessageSelectColumns = `id,account_id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,from_value,to_value,cc_value,bcc_value,subject,snippet,body_text,body_html_sanitized,raw_source,seen,flagged,answered,draft,has_attachments,importance,dkim_status,spf_status,dmarc_status,phishing_score,remote_images_blocked,remote_images_allowed,date_header,internal_date`

type recipientScore struct {
	models.RecipientSuggestion
	Score  int
	LastAt time.Time
}

func indexedMessageIDCandidates(accountID, id string) []string {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	scoped := mail.NormalizeIndexedMessageID(accountID, id)
	if scoped == "" {
		return nil
	}
	if scoped == id {
		return []string{scoped}
	}
	return []string{scoped, id}
}

func indexedThreadIDCandidates(accountID, id string) []string {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	scoped := mail.NormalizeIndexedThreadID(accountID, id)
	if scoped == "" {
		return nil
	}
	if scoped == id {
		return []string{scoped}
	}
	return []string{scoped, id}
}

func (s *Store) ListMailAccounts(ctx context.Context, userID string) ([]models.MailAccount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,display_name,login,secret_enc,imap_host,imap_port,imap_tls,imap_starttls,smtp_host,smtp_port,smtp_tls,smtp_starttls,is_default,status,last_sync_at,last_error,created_at,updated_at
		 FROM mail_accounts
		 WHERE user_id=?
		 ORDER BY is_default DESC, created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.MailAccount, 0, 8)
	for rows.Next() {
		item, err := scanMailAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListAllMailAccounts(ctx context.Context) ([]models.MailAccount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,display_name,login,secret_enc,imap_host,imap_port,imap_tls,imap_starttls,smtp_host,smtp_port,smtp_tls,smtp_starttls,is_default,status,last_sync_at,last_error,created_at,updated_at
		 FROM mail_accounts
		 ORDER BY updated_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.MailAccount, 0, 16)
	for rows.Next() {
		item, err := scanMailAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetMailAccountByID(ctx context.Context, userID, id string) (models.MailAccount, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,display_name,login,secret_enc,imap_host,imap_port,imap_tls,imap_starttls,smtp_host,smtp_port,smtp_tls,smtp_starttls,is_default,status,last_sync_at,last_error,created_at,updated_at
		 FROM mail_accounts
		 WHERE user_id=? AND id=?`,
		userID, id,
	)
	item, err := scanMailAccount(row)
	if err == sql.ErrNoRows {
		return models.MailAccount{}, ErrNotFound
	}
	if err != nil {
		return models.MailAccount{}, err
	}
	return item, nil
}

func (s *Store) CreateMailAccount(ctx context.Context, in models.MailAccount) (models.MailAccount, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	if strings.TrimSpace(in.Status) == "" {
		in.Status = "active"
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO mail_accounts(
			id,user_id,display_name,login,secret_enc,imap_host,imap_port,imap_tls,imap_starttls,smtp_host,smtp_port,smtp_tls,smtp_starttls,is_default,status,last_sync_at,last_error,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.UserID,
		in.DisplayName,
		in.Login,
		in.SecretEnc,
		in.IMAPHost,
		in.IMAPPort,
		boolToInt(in.IMAPTLS),
		boolToInt(in.IMAPStartTLS),
		in.SMTPHost,
		in.SMTPPort,
		boolToInt(in.SMTPTLS),
		boolToInt(in.SMTPStartTLS),
		boolToInt(in.IsDefault),
		in.Status,
		nullTimeValue(in.LastSyncAt),
		nullStringValue(in.LastError),
		in.CreatedAt,
		in.UpdatedAt,
	); err != nil {
		return models.MailAccount{}, err
	}
	if in.IsDefault {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE mail_accounts SET is_default=CASE WHEN id=? THEN 1 ELSE 0 END, updated_at=? WHERE user_id=?`,
			in.ID, now, in.UserID,
		)
	}
	return in, nil
}

func (s *Store) UpdateMailAccountSyncStatus(ctx context.Context, accountID string, at time.Time, lastErr string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE mail_accounts
		 SET last_sync_at=?, last_error=?, status=?, updated_at=?
		 WHERE id=?`,
		at,
		nullStringValue(lastErr),
		map[bool]string{true: "error", false: "active"}[strings.TrimSpace(lastErr) != ""],
		time.Now().UTC(),
		accountID,
	)
	return err
}

func (s *Store) UpdateMailAccount(ctx context.Context, in models.MailAccount) (models.MailAccount, error) {
	in.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE mail_accounts SET
		 display_name=?,login=?,secret_enc=?,imap_host=?,imap_port=?,imap_tls=?,imap_starttls=?,
		 smtp_host=?,smtp_port=?,smtp_tls=?,smtp_starttls=?,is_default=?,status=?,last_sync_at=?,last_error=?,updated_at=?
		 WHERE user_id=? AND id=?`,
		in.DisplayName,
		in.Login,
		in.SecretEnc,
		in.IMAPHost,
		in.IMAPPort,
		boolToInt(in.IMAPTLS),
		boolToInt(in.IMAPStartTLS),
		in.SMTPHost,
		in.SMTPPort,
		boolToInt(in.SMTPTLS),
		boolToInt(in.SMTPStartTLS),
		boolToInt(in.IsDefault),
		in.Status,
		nullTimeValue(in.LastSyncAt),
		nullStringValue(in.LastError),
		in.UpdatedAt,
		in.UserID,
		in.ID,
	)
	if err != nil {
		return models.MailAccount{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return models.MailAccount{}, ErrNotFound
	}
	if in.IsDefault {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE mail_accounts SET is_default=CASE WHEN id=? THEN 1 ELSE 0 END, updated_at=? WHERE user_id=?`,
			in.ID, in.UpdatedAt, in.UserID,
		)
	}
	return s.GetMailAccountByID(ctx, in.UserID, in.ID)
}

func (s *Store) DeleteMailAccount(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mail_accounts WHERE user_id=? AND id=?`, userID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetActiveMailAccount(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE mail_accounts SET is_default=CASE WHEN id=? THEN 1 ELSE 0 END, updated_at=? WHERE user_id=?`,
		id, time.Now().UTC(), userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListMailIdentities(ctx context.Context, accountID string) ([]models.MailIdentity, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,account_id,display_name,from_email,reply_to,signature_text,signature_html,is_default,created_at,updated_at
		 FROM mail_identities
		 WHERE account_id=?
		 ORDER BY is_default DESC, created_at ASC`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.MailIdentity, 0, 8)
	for rows.Next() {
		var item models.MailIdentity
		var isDefault int
		if err := rows.Scan(
			&item.ID,
			&item.AccountID,
			&item.DisplayName,
			&item.FromEmail,
			&item.ReplyTo,
			&item.SignatureText,
			&item.SignatureHTML,
			&isDefault,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.IsDefault = isDefault == 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateMailIdentity(ctx context.Context, in models.MailIdentity) (models.MailIdentity, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO mail_identities(id,account_id,display_name,from_email,reply_to,signature_text,signature_html,is_default,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		in.ID, in.AccountID, in.DisplayName, in.FromEmail, in.ReplyTo, in.SignatureText, in.SignatureHTML, boolToInt(in.IsDefault), in.CreatedAt, in.UpdatedAt,
	); err != nil {
		return models.MailIdentity{}, err
	}
	if in.IsDefault {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE mail_identities SET is_default=CASE WHEN id=? THEN 1 ELSE 0 END, updated_at=? WHERE account_id=?`,
			in.ID, now, in.AccountID,
		)
	}
	return in, nil
}

func (s *Store) GetMailIdentityByID(ctx context.Context, id string) (models.MailIdentity, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,account_id,display_name,from_email,reply_to,signature_text,signature_html,is_default,created_at,updated_at
		 FROM mail_identities
		 WHERE id=?`,
		id,
	)
	var out models.MailIdentity
	var isDefault int
	if err := row.Scan(
		&out.ID, &out.AccountID, &out.DisplayName, &out.FromEmail, &out.ReplyTo, &out.SignatureText, &out.SignatureHTML, &isDefault, &out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return models.MailIdentity{}, ErrNotFound
		}
		return models.MailIdentity{}, err
	}
	out.IsDefault = isDefault == 1
	return out, nil
}

func (s *Store) UpdateMailIdentity(ctx context.Context, in models.MailIdentity) (models.MailIdentity, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE mail_identities SET display_name=?,from_email=?,reply_to=?,signature_text=?,signature_html=?,is_default=?,updated_at=?
		 WHERE id=? AND account_id=?`,
		in.DisplayName, in.FromEmail, in.ReplyTo, in.SignatureText, in.SignatureHTML, boolToInt(in.IsDefault), now, in.ID, in.AccountID,
	)
	if err != nil {
		return models.MailIdentity{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.MailIdentity{}, ErrNotFound
	}
	if in.IsDefault {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE mail_identities SET is_default=CASE WHEN id=? THEN 1 ELSE 0 END, updated_at=? WHERE account_id=?`,
			in.ID, now, in.AccountID,
		)
	}
	return s.GetMailIdentityByID(ctx, in.ID)
}

func (s *Store) DeleteMailIdentity(ctx context.Context, accountID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mail_identities WHERE id=? AND account_id=?`, id, accountID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListMailboxMappings(ctx context.Context, accountID string) ([]models.MailboxMapping, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,account_id,role,mailbox_name,source,priority,created_at,updated_at
		 FROM mailbox_mappings
		 WHERE account_id=?
		 ORDER BY role ASC, priority ASC, mailbox_name ASC`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.MailboxMapping, 0, 8)
	for rows.Next() {
		var item models.MailboxMapping
		if err := rows.Scan(
			&item.ID, &item.AccountID, &item.Role, &item.MailboxName, &item.Source, &item.Priority, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertMailboxMapping(ctx context.Context, in models.MailboxMapping) (models.MailboxMapping, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mailbox_mappings(id,account_id,role,mailbox_name,source,priority,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET role=excluded.role, mailbox_name=excluded.mailbox_name, source=excluded.source, priority=excluded.priority, updated_at=excluded.updated_at`,
		in.ID, in.AccountID, in.Role, in.MailboxName, in.Source, in.Priority, coalesceTime(in.CreatedAt, now), in.UpdatedAt,
	)
	if err != nil {
		return models.MailboxMapping{}, err
	}
	return in, nil
}

func (s *Store) DeleteMailboxMapping(ctx context.Context, accountID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mailbox_mappings WHERE account_id=? AND id=?`, accountID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListIndexedMailboxCounts(ctx context.Context, accountID string) ([]mail.Mailbox, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT mailbox,
		        SUM(CASE WHEN seen=0 THEN 1 ELSE 0 END) AS unread_count,
		        COUNT(1) AS message_count
		   FROM message_index
		  WHERE account_id=?
		  GROUP BY mailbox
		  ORDER BY mailbox ASC`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]mail.Mailbox, 0, 12)
	for rows.Next() {
		var item mail.Mailbox
		if err := rows.Scan(&item.Name, &item.Unread, &item.Messages); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetUserPreferences(ctx context.Context, userID string) (models.UserPreferences, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT user_id,theme,density,layout_mode,keymap_json,remote_image_policy,timezone,page_size,grouping_mode,updated_at
		 FROM user_preferences
		 WHERE user_id=?`,
		userID,
	)
	var item models.UserPreferences
	err := row.Scan(
		&item.UserID,
		&item.Theme,
		&item.Density,
		&item.LayoutMode,
		&item.KeymapJSON,
		&item.RemoteImagePolicy,
		&item.Timezone,
		&item.PageSize,
		&item.GroupingMode,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return models.UserPreferences{
			UserID:            userID,
			Theme:             "machine-dark",
			Density:           "comfortable",
			LayoutMode:        "three-pane",
			KeymapJSON:        "{}",
			RemoteImagePolicy: "block",
			Timezone:          "UTC",
			PageSize:          50,
			GroupingMode:      "day",
			UpdatedAt:         time.Now().UTC(),
		}, nil
	}
	if err != nil {
		return models.UserPreferences{}, err
	}
	return item, nil
}

func (s *Store) UpsertUserPreferences(ctx context.Context, in models.UserPreferences) (models.UserPreferences, error) {
	in.UpdatedAt = time.Now().UTC()
	if strings.TrimSpace(in.Theme) == "" {
		in.Theme = "machine-dark"
	}
	if strings.TrimSpace(in.Density) == "" {
		in.Density = "comfortable"
	}
	if strings.TrimSpace(in.LayoutMode) == "" {
		in.LayoutMode = "three-pane"
	}
	if strings.TrimSpace(in.KeymapJSON) == "" {
		in.KeymapJSON = "{}"
	}
	if strings.TrimSpace(in.RemoteImagePolicy) == "" {
		in.RemoteImagePolicy = "block"
	}
	if strings.TrimSpace(in.Timezone) == "" {
		in.Timezone = "UTC"
	}
	if in.PageSize <= 0 {
		in.PageSize = 50
	}
	if strings.TrimSpace(in.GroupingMode) == "" {
		in.GroupingMode = "day"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_preferences(user_id,theme,density,layout_mode,keymap_json,remote_image_policy,timezone,page_size,grouping_mode,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   theme=excluded.theme,
		   density=excluded.density,
		   layout_mode=excluded.layout_mode,
		   keymap_json=excluded.keymap_json,
		   remote_image_policy=excluded.remote_image_policy,
		   timezone=excluded.timezone,
		   page_size=excluded.page_size,
		   grouping_mode=excluded.grouping_mode,
		   updated_at=excluded.updated_at`,
		in.UserID,
		in.Theme,
		in.Density,
		in.LayoutMode,
		in.KeymapJSON,
		in.RemoteImagePolicy,
		in.Timezone,
		in.PageSize,
		in.GroupingMode,
		in.UpdatedAt,
	)
	if err != nil {
		return models.UserPreferences{}, err
	}
	return in, nil
}

func (s *Store) ListSavedSearches(ctx context.Context, userID string) ([]models.SavedSearch, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,account_id,name,filters_json,pinned,created_at,updated_at
		 FROM saved_searches
		 WHERE user_id=?
		 ORDER BY pinned DESC, name ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SavedSearch, 0, 16)
	for rows.Next() {
		var item models.SavedSearch
		var pinned int
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.AccountID, &item.Name, &item.FiltersJSON, &pinned, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Pinned = pinned == 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) CreateSavedSearch(ctx context.Context, in models.SavedSearch) (models.SavedSearch, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO saved_searches(id,user_id,account_id,name,filters_json,pinned,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		in.ID, in.UserID, in.AccountID, in.Name, in.FiltersJSON, boolToInt(in.Pinned), in.CreatedAt, in.UpdatedAt,
	)
	if err != nil {
		return models.SavedSearch{}, err
	}
	return in, nil
}

func (s *Store) UpdateSavedSearch(ctx context.Context, in models.SavedSearch) (models.SavedSearch, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE saved_searches
		 SET name=?,filters_json=?,pinned=?,account_id=?,updated_at=?
		 WHERE user_id=? AND id=?`,
		in.Name, in.FiltersJSON, boolToInt(in.Pinned), in.AccountID, now, in.UserID, in.ID,
	)
	if err != nil {
		return models.SavedSearch{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.SavedSearch{}, ErrNotFound
	}
	in.UpdatedAt = now
	return in, nil
}

func (s *Store) DeleteSavedSearch(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM saved_searches WHERE user_id=? AND id=?`, userID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListDrafts(ctx context.Context, userID, accountID string, limit, offset int) ([]models.Draft, int, error) {
	where := []string{"user_id=?", "status<>'sent'"}
	args := []any{userID}
	if strings.TrimSpace(accountID) != "" {
		where = append(where, "account_id=?")
		args = append(args, accountID)
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(1) FROM drafts WHERE %s`, whereSQL), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit = clampLimit(limit)
	offset = clampOffset(offset)
	query := fmt.Sprintf(
		`SELECT id,user_id,account_id,identity_id,compose_mode,context_message_id,context_account_id,from_mode,from_manual,client_state_json,to_value,cc_value,bcc_value,subject,body_text,body_html,attachments_json,crypto_options_json,send_mode,scheduled_for,status,last_send_error,created_at,updated_at
		 FROM drafts
		 WHERE %s
		 ORDER BY updated_at DESC
		 LIMIT ? OFFSET ?`,
		whereSQL,
	)
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]models.Draft, 0, limit)
	for rows.Next() {
		item, err := scanDraft(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) GetDraftByID(ctx context.Context, userID, id string) (models.Draft, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,account_id,identity_id,compose_mode,context_message_id,context_account_id,from_mode,from_manual,client_state_json,to_value,cc_value,bcc_value,subject,body_text,body_html,attachments_json,crypto_options_json,send_mode,scheduled_for,status,last_send_error,created_at,updated_at
		 FROM drafts
		 WHERE user_id=? AND id=?`,
		userID, id,
	)
	item, err := scanDraft(row)
	if err == sql.ErrNoRows {
		return models.Draft{}, ErrNotFound
	}
	if err != nil {
		return models.Draft{}, err
	}
	return item, nil
}

func (s *Store) CreateDraft(ctx context.Context, in models.Draft) (models.Draft, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	if strings.TrimSpace(in.Status) == "" {
		in.Status = "draft"
	}
	in.AttachmentsJSON = normalizeDraftAttachmentsJSON(in.AttachmentsJSON)
	in.LastSendError = strings.TrimSpace(in.LastSendError)
	in.CreatedAt = now
	in.UpdatedAt = now
	if strings.TrimSpace(in.ComposeMode) == "" {
		in.ComposeMode = "send"
	}
	if strings.TrimSpace(in.FromMode) == "" {
		in.FromMode = "default"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO drafts(
		  id,user_id,account_id,identity_id,compose_mode,context_message_id,context_account_id,from_mode,from_manual,client_state_json,to_value,cc_value,bcc_value,subject,body_text,body_html,attachments_json,crypto_options_json,send_mode,scheduled_for,status,last_send_error,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.UserID,
		nullStringValue(in.AccountID),
		in.IdentityID,
		in.ComposeMode,
		in.ContextMessageID,
		in.ContextAccountID,
		in.FromMode,
		in.FromManual,
		in.ClientStateJSON,
		in.ToValue,
		in.CCValue,
		in.BCCValue,
		in.Subject,
		in.BodyText,
		in.BodyHTML,
		in.AttachmentsJSON,
		in.CryptoOptions,
		in.SendMode,
		nullTimeValue(in.ScheduledFor),
		in.Status,
		in.LastSendError,
		in.CreatedAt,
		in.UpdatedAt,
	)
	if err != nil {
		return models.Draft{}, err
	}
	snap, _ := json.Marshal(in)
	_, _ = s.AddDraftVersion(ctx, in.ID, string(snap))
	return in, nil
}

func (s *Store) UpdateDraft(ctx context.Context, in models.Draft) (models.Draft, error) {
	in.UpdatedAt = time.Now().UTC()
	if strings.TrimSpace(in.ComposeMode) == "" {
		in.ComposeMode = "send"
	}
	if strings.TrimSpace(in.FromMode) == "" {
		in.FromMode = "default"
	}
	in.AttachmentsJSON = normalizeDraftAttachmentsJSON(in.AttachmentsJSON)
	in.LastSendError = strings.TrimSpace(in.LastSendError)
	res, err := s.db.ExecContext(ctx,
		`UPDATE drafts SET
		  account_id=?,identity_id=?,compose_mode=?,context_message_id=?,context_account_id=?,from_mode=?,from_manual=?,client_state_json=?,to_value=?,cc_value=?,bcc_value=?,subject=?,body_text=?,body_html=?,attachments_json=?,crypto_options_json=?,send_mode=?,scheduled_for=?,status=?,last_send_error=?,updated_at=?
		 WHERE user_id=? AND id=?`,
		nullStringValue(in.AccountID),
		in.IdentityID,
		in.ComposeMode,
		in.ContextMessageID,
		in.ContextAccountID,
		in.FromMode,
		in.FromManual,
		in.ClientStateJSON,
		in.ToValue,
		in.CCValue,
		in.BCCValue,
		in.Subject,
		in.BodyText,
		in.BodyHTML,
		in.AttachmentsJSON,
		in.CryptoOptions,
		in.SendMode,
		nullTimeValue(in.ScheduledFor),
		in.Status,
		in.LastSendError,
		in.UpdatedAt,
		in.UserID,
		in.ID,
	)
	if err != nil {
		return models.Draft{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.Draft{}, ErrNotFound
	}
	snap, _ := json.Marshal(in)
	_, _ = s.AddDraftVersion(ctx, in.ID, string(snap))
	return s.GetDraftByID(ctx, in.UserID, in.ID)
}

func (s *Store) DeleteDraft(ctx context.Context, userID, draftID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM draft_versions WHERE draft_id=?`, draftID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM draft_attachments WHERE draft_id=? AND user_id=?`, draftID, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM scheduled_send_queue WHERE draft_id=?`, draftID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM drafts WHERE user_id=? AND id=?`, userID, draftID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (s *Store) AddDraftVersion(ctx context.Context, draftID, snapshotJSON string) (models.DraftVersion, error) {
	var versionNo int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_no),0)+1 FROM draft_versions WHERE draft_id=?`, draftID).Scan(&versionNo); err != nil {
		return models.DraftVersion{}, err
	}
	now := time.Now().UTC()
	item := models.DraftVersion{
		ID:           uuid.NewString(),
		DraftID:      draftID,
		VersionNo:    versionNo,
		SnapshotJSON: snapshotJSON,
		CreatedAt:    now,
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO draft_versions(id,draft_id,version_no,snapshot_json,created_at) VALUES(?,?,?,?,?)`,
		item.ID, item.DraftID, item.VersionNo, item.SnapshotJSON, item.CreatedAt,
	); err != nil {
		return models.DraftVersion{}, err
	}
	_, _ = s.db.ExecContext(ctx,
		`DELETE FROM draft_versions
		 WHERE draft_id=?
		   AND id IN (
		     SELECT id FROM draft_versions WHERE draft_id=? ORDER BY version_no DESC LIMIT -1 OFFSET 20
		   )`,
		draftID,
		draftID,
	)
	return item, nil
}

func (s *Store) ListDraftVersions(ctx context.Context, draftID string, limit int) ([]models.DraftVersion, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,draft_id,version_no,snapshot_json,created_at
		 FROM draft_versions
		 WHERE draft_id=?
		 ORDER BY version_no DESC
		 LIMIT ?`,
		draftID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.DraftVersion, 0, limit)
	for rows.Next() {
		var item models.DraftVersion
		if err := rows.Scan(&item.ID, &item.DraftID, &item.VersionNo, &item.SnapshotJSON, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SetDraftStatus(ctx context.Context, userID, draftID, status string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE drafts SET status=?, updated_at=? WHERE user_id=? AND id=?`,
		strings.TrimSpace(status), time.Now().UTC(), userID, draftID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetDraftSendState(ctx context.Context, userID, draftID, status, lastSendError string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE drafts SET status=?, last_send_error=?, updated_at=? WHERE user_id=? AND id=?`,
		strings.TrimSpace(status), strings.TrimSpace(lastSendError), time.Now().UTC(), userID, draftID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) NextDraftAttachmentSortOrder(ctx context.Context, userID, draftID string) (int, error) {
	var next int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM draft_attachments WHERE user_id=? AND draft_id=?`,
		userID, draftID,
	).Scan(&next); err != nil {
		return 0, err
	}
	if next < 0 {
		next = 0
	}
	return next, nil
}

func (s *Store) CreateDraftAttachment(ctx context.Context, in models.DraftAttachment) (models.DraftAttachment, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	if strings.TrimSpace(in.ContentType) == "" {
		in.ContentType = "application/octet-stream"
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	in.SizeBytes = int64(len(in.Data))
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO draft_attachments(
		  id,draft_id,user_id,filename,content_type,size_bytes,inline_part,content_id,sort_order,data,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.DraftID,
		in.UserID,
		in.Filename,
		in.ContentType,
		in.SizeBytes,
		boolToInt(in.InlinePart),
		in.ContentID,
		in.SortOrder,
		in.Data,
		in.CreatedAt,
		in.UpdatedAt,
	)
	if err != nil {
		return models.DraftAttachment{}, err
	}
	return in, nil
}

func (s *Store) ListDraftAttachments(ctx context.Context, userID, draftID string) ([]models.DraftAttachment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,draft_id,user_id,filename,content_type,size_bytes,inline_part,content_id,sort_order,data,created_at,updated_at
		 FROM draft_attachments
		 WHERE user_id=? AND draft_id=?
		 ORDER BY sort_order ASC, created_at ASC`,
		userID, draftID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.DraftAttachment, 0, 8)
	for rows.Next() {
		item, err := scanDraftAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetDraftAttachmentByID(ctx context.Context, userID, draftID, attachmentID string) (models.DraftAttachment, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,draft_id,user_id,filename,content_type,size_bytes,inline_part,content_id,sort_order,data,created_at,updated_at
		 FROM draft_attachments
		 WHERE user_id=? AND draft_id=? AND id=?`,
		userID, draftID, attachmentID,
	)
	item, err := scanDraftAttachment(row)
	if err == sql.ErrNoRows {
		return models.DraftAttachment{}, ErrNotFound
	}
	if err != nil {
		return models.DraftAttachment{}, err
	}
	return item, nil
}

func (s *Store) DeleteDraftAttachment(ctx context.Context, userID, draftID, attachmentID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM draft_attachments WHERE user_id=? AND draft_id=? AND id=?`,
		userID, draftID, attachmentID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) QueueScheduledSend(ctx context.Context, draft models.Draft) error {
	if strings.TrimSpace(draft.ID) == "" || strings.TrimSpace(draft.UserID) == "" || strings.TrimSpace(draft.AccountID) == "" {
		return fmt.Errorf("draft id, user id, and account id are required")
	}
	now := time.Now().UTC()
	dueAt := draft.ScheduledFor.UTC()
	if dueAt.IsZero() {
		dueAt = now
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE scheduled_send_queue
		 SET due_at=?, state='queued', next_retry_at=NULL, last_error=NULL, updated_at=?
		 WHERE draft_id=?`,
		dueAt, now, draft.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO scheduled_send_queue(
			  id,draft_id,user_id,account_id,due_at,state,retry_count,next_retry_at,last_error,created_at,updated_at
			) VALUES(?,?,?,?,?,'queued',0,NULL,NULL,?,?)`,
			uuid.NewString(), draft.ID, draft.UserID, draft.AccountID, dueAt, now, now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListDueScheduledSends(ctx context.Context, now time.Time, limit int) ([]models.ScheduledSendQueueItem, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,draft_id,user_id,account_id,due_at,state,retry_count,next_retry_at,COALESCE(last_error,''),created_at,updated_at
		 FROM scheduled_send_queue
		 WHERE state IN ('queued','retrying')
		   AND due_at <= ?
		   AND (next_retry_at IS NULL OR next_retry_at <= ?)
		 ORDER BY due_at ASC, created_at ASC
		 LIMIT ?`,
		now, now, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ScheduledSendQueueItem, 0, limit)
	for rows.Next() {
		var item models.ScheduledSendQueueItem
		var nextRetryAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.DraftID,
			&item.UserID,
			&item.AccountID,
			&item.DueAt,
			&item.State,
			&item.RetryCount,
			&nextRetryAt,
			&item.LastError,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if nextRetryAt.Valid {
			item.NextRetryAt = nextRetryAt.Time
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) MarkScheduledSendRetry(ctx context.Context, queueID string, retryCount int, nextRetryAt time.Time, lastErr string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_send_queue
		 SET state='retrying', retry_count=?, next_retry_at=?, last_error=?, updated_at=?
		 WHERE id=?`,
		retryCount, nextRetryAt.UTC(), nullStringValue(lastErr), time.Now().UTC(), queueID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkScheduledSendSent(ctx context.Context, queueID string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_send_queue
		 SET state='sent', next_retry_at=NULL, last_error=NULL, updated_at=?
		 WHERE id=?`,
		time.Now().UTC(), queueID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkScheduledSendFailed(ctx context.Context, queueID, lastErr string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE scheduled_send_queue
		 SET state='failed', next_retry_at=NULL, last_error=?, updated_at=?
		 WHERE id=?`,
		nullStringValue(lastErr), time.Now().UTC(), queueID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpsertIndexedMessage(ctx context.Context, in models.IndexedMessage) (models.IndexedMessage, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.ID = mail.NormalizeIndexedMessageID(in.AccountID, in.ID)
	if strings.TrimSpace(in.ThreadID) == "" {
		in.ThreadID = in.ID
	}
	in.ThreadID = mail.NormalizeIndexedThreadID(in.AccountID, in.ThreadID)
	if in.DateHeader.IsZero() {
		in.DateHeader = now
	}
	if in.InternalDate.IsZero() {
		in.InternalDate = in.DateHeader
	}
	in.MessageIDHeader = mail.NormalizeMessageIDHeader(in.MessageIDHeader)
	in.InReplyToHeader = mail.NormalizeMessageIDHeader(in.InReplyToHeader)
	in.ReferencesHeader = mail.FormatMessageIDList(mail.ParseMessageIDList(in.ReferencesHeader))
	participants := []string{}
	if fromValue := strings.TrimSpace(in.FromValue); fromValue != "" {
		participants = append(participants, fromValue)
	}
	participantsJSON, _ := json.Marshal(participants)
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO thread_index(
		  id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO NOTHING`,
		in.ThreadID,
		in.AccountID,
		in.Mailbox,
		firstNonEmptyString(strings.TrimSpace(in.Subject), "(no subject)"),
		string(participantsJSON),
		1,
		boolToInt(!in.Seen),
		boolToInt(in.HasAttachments),
		boolToInt(in.Flagged),
		in.Importance,
		in.ID,
		in.InternalDate,
		now,
	); err != nil {
		return models.IndexedMessage{}, err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO message_index(
		  id,account_id,mailbox,uid,thread_id,message_id_header,in_reply_to_header,references_header,
		  from_value,to_value,cc_value,bcc_value,subject,snippet,body_text,body_html_sanitized,raw_source,
		  seen,flagged,answered,draft,has_attachments,importance,dkim_status,spf_status,dmarc_status,phishing_score,
		  remote_images_blocked,remote_images_allowed,date_header,internal_date,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		  mailbox=excluded.mailbox,
		  uid=excluded.uid,
		  thread_id=excluded.thread_id,
		  message_id_header=excluded.message_id_header,
		  in_reply_to_header=excluded.in_reply_to_header,
		  references_header=excluded.references_header,
		  from_value=excluded.from_value,
		  to_value=excluded.to_value,
		  cc_value=excluded.cc_value,
		  bcc_value=excluded.bcc_value,
		  subject=excluded.subject,
		  snippet=excluded.snippet,
		  body_text=excluded.body_text,
		  body_html_sanitized=excluded.body_html_sanitized,
		  raw_source=excluded.raw_source,
		  seen=excluded.seen,
		  flagged=excluded.flagged,
		  answered=excluded.answered,
		  draft=excluded.draft,
		  has_attachments=excluded.has_attachments,
		  importance=excluded.importance,
		  dkim_status=excluded.dkim_status,
		  spf_status=excluded.spf_status,
		  dmarc_status=excluded.dmarc_status,
		  phishing_score=excluded.phishing_score,
		  remote_images_blocked=excluded.remote_images_blocked,
		  remote_images_allowed=excluded.remote_images_allowed,
		  date_header=excluded.date_header,
		  internal_date=excluded.internal_date,
		  updated_at=excluded.updated_at`,
		in.ID,
		in.AccountID,
		in.Mailbox,
		in.UID,
		in.ThreadID,
		in.MessageIDHeader,
		in.InReplyToHeader,
		in.ReferencesHeader,
		in.FromValue,
		in.ToValue,
		in.CCValue,
		in.BCCValue,
		in.Subject,
		in.Snippet,
		in.BodyText,
		in.BodyHTMLSanitized,
		in.RawSource,
		boolToInt(in.Seen),
		boolToInt(in.Flagged),
		boolToInt(in.Answered),
		boolToInt(in.Draft),
		boolToInt(in.HasAttachments),
		in.Importance,
		firstNonEmptyString(in.DKIMStatus, "unknown"),
		firstNonEmptyString(in.SPFStatus, "unknown"),
		firstNonEmptyString(in.DMARCStatus, "unknown"),
		in.PhishingScore,
		boolToInt(in.RemoteImagesBlocked),
		boolToInt(in.RemoteImagesAllowed),
		in.DateHeader.UTC(),
		in.InternalDate.UTC(),
		now,
		now,
	)
	if err != nil {
		return models.IndexedMessage{}, err
	}
	return s.GetIndexedMessageByID(ctx, in.AccountID, in.ID)
}

func (s *Store) ReplaceIndexedAttachments(ctx context.Context, accountID, messageID string, items []models.IndexedAttachment) error {
	messageID = mail.NormalizeIndexedMessageID(accountID, messageID)
	if strings.TrimSpace(messageID) == "" {
		return fmt.Errorf("message id is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM attachment_index WHERE account_id=? AND message_id=?`, accountID, messageID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, item := range items {
		if strings.TrimSpace(item.ID) == "" {
			item.ID = uuid.NewString()
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = now
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO attachment_index(id,message_id,account_id,filename,content_type,size_bytes,inline_part,created_at)
			 VALUES(?,?,?,?,?,?,?,?)`,
			item.ID,
			messageID,
			accountID,
			item.Filename,
			item.ContentType,
			item.SizeBytes,
			boolToInt(item.InlinePart),
			item.CreatedAt,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RebuildThreadIndex(ctx context.Context, accountID string) error {
	rows, err := s.db.QueryContext(ctx,
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
	threads := make(map[string]*threadAgg, 128)
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
				AccountID:    accountID,
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	for _, agg := range threads {
		participants := make([]string, 0, len(agg.Participants))
		for p := range agg.Participants {
			participants = append(participants, p)
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
	if len(threads) == 0 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM thread_index WHERE account_id=?`, accountID); err != nil {
			return err
		}
		return tx.Commit()
	}
	threadIDs := make([]string, 0, len(threads))
	for threadID := range threads {
		threadIDs = append(threadIDs, threadID)
	}
	sort.Strings(threadIDs)
	deleteArgs := make([]any, 0, len(threadIDs)+1)
	deleteArgs = append(deleteArgs, accountID)
	for _, threadID := range threadIDs {
		deleteArgs = append(deleteArgs, threadID)
	}
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM thread_index WHERE account_id=? AND id NOT IN (%s)`, placeholders(len(threadIDs))),
		deleteArgs...,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListThreads(ctx context.Context, accountID, mailbox, sort string, limit, offset int) ([]models.ThreadSummary, int, error) {
	where := []string{"account_id=?"}
	args := []any{accountID}
	if strings.TrimSpace(mailbox) != "" {
		where = append(where, `EXISTS (
			SELECT 1
			FROM message_index mi
			WHERE mi.account_id=thread_index.account_id
			  AND mi.thread_id=thread_index.id
			  AND mi.mailbox=?
		)`)
		args = append(args, mailbox)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(1) FROM thread_index WHERE %s`, whereSQL), args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	orderBy := "latest_at DESC"
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "oldest":
		orderBy = "latest_at ASC"
	case "subject":
		orderBy = "subject_norm ASC"
	case "unread":
		orderBy = "unread_count DESC, latest_at DESC"
	case "flagged":
		orderBy = "has_flagged DESC, latest_at DESC"
	}
	limit = clampLimit(limit)
	offset = clampOffset(offset)

	query := fmt.Sprintf(
		`SELECT id,account_id,mailbox,subject_norm,participants_json,message_count,unread_count,has_attachments,has_flagged,importance,latest_message_id,latest_at
		 FROM thread_index
		 WHERE %s
		 ORDER BY %s
		 LIMIT ? OFFSET ?`,
		whereSQL,
		orderBy,
	)
	args = append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]models.ThreadSummary, 0, limit)
	for rows.Next() {
		var item models.ThreadSummary
		var participantsJSON string
		var hasAttachments, hasFlagged int
		if err := rows.Scan(
			&item.ID,
			&item.AccountID,
			&item.Mailbox,
			&item.SubjectNorm,
			&participantsJSON,
			&item.MessageCount,
			&item.UnreadCount,
			&hasAttachments,
			&hasFlagged,
			&item.Importance,
			&item.LatestMessage,
			&item.LatestAt,
		); err != nil {
			return nil, 0, err
		}
		item.HasAttachments = hasAttachments == 1
		item.HasFlagged = hasFlagged == 1
		_ = json.Unmarshal([]byte(participantsJSON), &item.Participants)
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) ListMessagesByThread(ctx context.Context, accountID, threadID string, limit, offset int) ([]models.IndexedMessage, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)
	candidates := indexedThreadIDCandidates(accountID, threadID)
	if len(candidates) == 0 {
		return nil, ErrNotFound
	}
	for i, candidate := range candidates {
		rows, err := s.db.QueryContext(ctx,
			fmt.Sprintf(`SELECT %s
			 FROM message_index
			 WHERE account_id=? AND thread_id=?
			 ORDER BY date_header DESC
			 LIMIT ? OFFSET ?`, indexedMessageSelectColumns),
			accountID, candidate, limit, offset,
		)
		if err != nil {
			return nil, err
		}
		out := make([]models.IndexedMessage, 0, limit)
		for rows.Next() {
			item, err := scanIndexedMessage(rows)
			if err != nil {
				_ = rows.Close()
				return nil, err
			}
			out = append(out, item)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
		if len(out) > 0 || i == len(candidates)-1 {
			return out, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) GetIndexedMessageByID(ctx context.Context, accountID, id string) (models.IndexedMessage, error) {
	candidates := indexedMessageIDCandidates(accountID, id)
	if len(candidates) == 0 {
		return models.IndexedMessage{}, ErrNotFound
	}
	for _, candidate := range candidates {
		row := s.db.QueryRowContext(ctx,
			fmt.Sprintf(`SELECT %s
			 FROM message_index
			 WHERE account_id=? AND id=?`, indexedMessageSelectColumns),
			accountID, candidate,
		)
		item, err := scanIndexedMessage(row)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return models.IndexedMessage{}, err
		}
		return item, nil
	}
	return models.IndexedMessage{}, ErrNotFound
}

func normalizeMailboxFilters(mailboxes []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(mailboxes))
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
		out = append(out, trimmed)
	}
	return out
}

func mailboxFilterClause(column string, mailboxes []string, args *[]any) string {
	normalized := normalizeMailboxFilters(mailboxes)
	switch len(normalized) {
	case 0:
		return ""
	case 1:
		*args = append(*args, normalized[0])
		return column + "=?"
	default:
		for _, mailbox := range normalized {
			*args = append(*args, mailbox)
		}
		return fmt.Sprintf("%s IN (%s)", column, placeholders(len(normalized)))
	}
}

func indexedMessageSortTime(item models.IndexedMessage) time.Time {
	if !item.DateHeader.IsZero() {
		return item.DateHeader
	}
	return item.InternalDate
}

func indexedMessageLess(a, b models.IndexedMessage, sortOrder string) bool {
	switch strings.ToLower(strings.TrimSpace(sortOrder)) {
	case "oldest":
		at := indexedMessageSortTime(a)
		bt := indexedMessageSortTime(b)
		if !at.Equal(bt) {
			return at.Before(bt)
		}
	case "subject":
		as := strings.ToLower(strings.TrimSpace(a.Subject))
		bs := strings.ToLower(strings.TrimSpace(b.Subject))
		if as != bs {
			return as < bs
		}
		at := indexedMessageSortTime(a)
		bt := indexedMessageSortTime(b)
		if !at.Equal(bt) {
			return at.After(bt)
		}
	case "sender":
		af := strings.ToLower(strings.TrimSpace(a.FromValue))
		bf := strings.ToLower(strings.TrimSpace(b.FromValue))
		if af != bf {
			return af < bf
		}
		at := indexedMessageSortTime(a)
		bt := indexedMessageSortTime(b)
		if !at.Equal(bt) {
			return at.After(bt)
		}
	default:
		at := indexedMessageSortTime(a)
		bt := indexedMessageSortTime(b)
		if !at.Equal(bt) {
			return at.After(bt)
		}
	}
	if a.AccountID != b.AccountID {
		return a.AccountID < b.AccountID
	}
	return a.ID < b.ID
}

func normalizeIndexedAccountIDs(accountIDs []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		trimmed := strings.TrimSpace(accountID)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func (s *Store) listIndexedMessagesForMailboxes(ctx context.Context, accountID string, mailboxes []string, view, sort string, limit, offset int) ([]models.IndexedMessage, int, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)

	where := []string{"account_id=?"}
	args := []any{accountID}
	if clause := mailboxFilterClause("mailbox", mailboxes, &args); clause != "" {
		where = append(where, clause)
	}
	switch strings.ToLower(strings.TrimSpace(view)) {
	case "unread":
		where = append(where, "seen=0")
	case "flagged":
		where = append(where, "flagged=1")
	case "attachments":
		where = append(where, "has_attachments=1")
	}
	whereSQL := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(1) FROM message_index WHERE %s`, whereSQL), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "date_header DESC"
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "oldest":
		orderBy = "date_header ASC"
	case "subject":
		orderBy = "subject ASC, date_header DESC"
	case "sender":
		orderBy = "from_value ASC, date_header DESC"
	}

	query := fmt.Sprintf(
		`SELECT %s
		 FROM message_index
		 WHERE %s
		 ORDER BY %s
		 LIMIT ? OFFSET ?`,
		indexedMessageSelectColumns,
		whereSQL,
		orderBy,
	)
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := s.db.QueryContext(ctx, query, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]models.IndexedMessage, 0, limit)
	for rows.Next() {
		item, err := scanIndexedMessage(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) ListIndexedMessages(ctx context.Context, accountID, mailbox, view, sort string, limit, offset int) ([]models.IndexedMessage, int, error) {
	mailboxes := []string(nil)
	if trimmed := strings.TrimSpace(mailbox); trimmed != "" {
		mailboxes = []string{trimmed}
	}
	return s.listIndexedMessagesForMailboxes(ctx, accountID, mailboxes, view, sort, limit, offset)
}

func (s *Store) GetIndexedMessageAttachments(ctx context.Context, accountID, messageID string) ([]models.IndexedAttachment, error) {
	candidates := indexedMessageIDCandidates(accountID, messageID)
	if len(candidates) == 0 {
		return nil, ErrNotFound
	}
	for i, candidate := range candidates {
		rows, err := s.db.QueryContext(ctx,
			`SELECT id,message_id,account_id,filename,content_type,size_bytes,inline_part,created_at
			 FROM attachment_index
			 WHERE account_id=? AND message_id=?
			 ORDER BY created_at ASC`,
			accountID, candidate,
		)
		if err != nil {
			return nil, err
		}
		out := make([]models.IndexedAttachment, 0, 4)
		for rows.Next() {
			var item models.IndexedAttachment
			var inlinePart int
			if err := rows.Scan(
				&item.ID,
				&item.MessageID,
				&item.AccountID,
				&item.Filename,
				&item.ContentType,
				&item.SizeBytes,
				&inlinePart,
				&item.CreatedAt,
			); err != nil {
				_ = rows.Close()
				return nil, err
			}
			item.InlinePart = inlinePart == 1
			out = append(out, item)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
		if len(out) > 0 || i == len(candidates)-1 {
			return out, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) searchIndexedMessagesForMailboxes(ctx context.Context, accountID string, mailboxes []string, query string, limit, offset int) ([]models.IndexedMessage, int, error) {
	limit = clampLimit(limit)
	offset = clampOffset(offset)
	if strings.TrimSpace(query) == "" {
		query = "*"
	}
	totalQuery := `SELECT COUNT(1)
		FROM message_search_fts f
		JOIN message_index m ON m.id = f.message_id
		WHERE f.account_id=?`
	args := []any{accountID}
	if clause := mailboxFilterClause("f.mailbox", mailboxes, &args); clause != "" {
		totalQuery += ` AND ` + clause
	}
	totalQuery += ` AND message_search_fts MATCH ?`
	args = append(args, query)

	var total int
	if err := s.db.QueryRowContext(ctx, totalQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	listQuery := `SELECT m.id,m.account_id,m.mailbox,m.uid,m.thread_id,m.message_id_header,m.in_reply_to_header,m.references_header,m.from_value,m.to_value,m.cc_value,m.bcc_value,m.subject,m.snippet,m.body_text,m.body_html_sanitized,m.raw_source,m.seen,m.flagged,m.answered,m.draft,m.has_attachments,m.importance,m.dkim_status,m.spf_status,m.dmarc_status,m.phishing_score,m.remote_images_blocked,m.remote_images_allowed,m.date_header,m.internal_date
		FROM message_search_fts f
		JOIN message_index m ON m.id = f.message_id
		WHERE f.account_id=?`
	listArgs := []any{accountID}
	if clause := mailboxFilterClause("f.mailbox", mailboxes, &listArgs); clause != "" {
		listQuery += ` AND ` + clause
	}
	listQuery += ` AND message_search_fts MATCH ?
		ORDER BY m.date_header DESC
		LIMIT ? OFFSET ?`
	listArgs = append(listArgs, query, limit, offset)

	rows, err := s.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]models.IndexedMessage, 0, limit)
	for rows.Next() {
		item, err := scanIndexedMessage(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, item)
	}
	return out, total, rows.Err()
}

func (s *Store) SearchIndexedMessages(ctx context.Context, accountID, mailbox, query string, limit, offset int) ([]models.IndexedMessage, int, error) {
	mailboxes := []string(nil)
	if trimmed := strings.TrimSpace(mailbox); trimmed != "" {
		mailboxes = []string{trimmed}
	}
	return s.searchIndexedMessagesForMailboxes(ctx, accountID, mailboxes, query, limit, offset)
}

func (s *Store) ListIndexedMessagesByAccounts(ctx context.Context, accountIDs []string, mailboxesByAccount map[string][]string, view, sortOrder string, limit, offset int) ([]models.IndexedMessage, int, error) {
	normalizedAccounts := normalizeIndexedAccountIDs(accountIDs)
	limit = clampLimit(limit)
	offset = clampOffset(offset)
	if len(normalizedAccounts) == 0 {
		return []models.IndexedMessage{}, 0, nil
	}
	fetchLimit := limit + offset
	if fetchLimit < limit {
		fetchLimit = limit
	}
	total := 0
	merged := make([]models.IndexedMessage, 0, fetchLimit*len(normalizedAccounts))
	for _, accountID := range normalizedAccounts {
		resolvedMailboxes, filtered := mailboxesByAccount[accountID]
		resolvedMailboxes = normalizeMailboxFilters(resolvedMailboxes)
		if filtered && len(resolvedMailboxes) == 0 {
			continue
		}
		items, count, err := s.listIndexedMessagesForMailboxes(ctx, accountID, resolvedMailboxes, view, sortOrder, fetchLimit, 0)
		if err != nil {
			return nil, 0, err
		}
		total += count
		merged = append(merged, items...)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return indexedMessageLess(merged[i], merged[j], sortOrder)
	})
	if offset >= len(merged) {
		return []models.IndexedMessage{}, total, nil
	}
	end := offset + limit
	if end > len(merged) {
		end = len(merged)
	}
	return append([]models.IndexedMessage(nil), merged[offset:end]...), total, nil
}

func (s *Store) SearchIndexedMessagesByAccounts(ctx context.Context, accountIDs []string, mailboxesByAccount map[string][]string, query string, limit, offset int) ([]models.IndexedMessage, int, error) {
	normalizedAccounts := normalizeIndexedAccountIDs(accountIDs)
	limit = clampLimit(limit)
	offset = clampOffset(offset)
	if len(normalizedAccounts) == 0 {
		return []models.IndexedMessage{}, 0, nil
	}
	fetchLimit := limit + offset
	if fetchLimit < limit {
		fetchLimit = limit
	}
	total := 0
	merged := make([]models.IndexedMessage, 0, fetchLimit*len(normalizedAccounts))
	for _, accountID := range normalizedAccounts {
		resolvedMailboxes, filtered := mailboxesByAccount[accountID]
		resolvedMailboxes = normalizeMailboxFilters(resolvedMailboxes)
		if filtered && len(resolvedMailboxes) == 0 {
			continue
		}
		items, count, err := s.searchIndexedMessagesForMailboxes(ctx, accountID, resolvedMailboxes, query, fetchLimit, 0)
		if err != nil {
			return nil, 0, err
		}
		total += count
		merged = append(merged, items...)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return indexedMessageLess(merged[i], merged[j], "")
	})
	if offset >= len(merged) {
		return []models.IndexedMessage{}, total, nil
	}
	end := offset + limit
	if end > len(merged) {
		end = len(merged)
	}
	return append([]models.IndexedMessage(nil), merged[offset:end]...), total, nil
}

func (s *Store) SuggestRecipients(ctx context.Context, accountID string, selfEmails []string, query string, limit int) ([]models.RecipientSuggestion, error) {
	if limit <= 0 {
		limit = 8
	}
	if limit > 25 {
		limit = 25
	}
	sampleLimit := limit * 40
	if sampleLimit < 200 {
		sampleLimit = 200
	}
	if sampleLimit > 600 {
		sampleLimit = 600
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT from_value,to_value,cc_value,bcc_value,internal_date
		 FROM message_index
		 WHERE account_id=?
		 ORDER BY internal_date DESC
		 LIMIT ?`,
		accountID,
		sampleLimit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	selfSet := map[string]struct{}{}
	for _, value := range selfEmails {
		email := normalizeSuggestionEmail(value)
		if email == "" {
			continue
		}
		selfSet[email] = struct{}{}
	}
	suggestions := map[string]recipientScore{}
	index := 0
	for rows.Next() {
		var fromValue string
		var toValue string
		var ccValue string
		var bccValue string
		var internalDate time.Time
		if err := rows.Scan(&fromValue, &toValue, &ccValue, &bccValue, &internalDate); err != nil {
			return nil, err
		}
		recencyWeight := sampleLimit - index
		index++

		sender, senderOK := firstLooseAddress(fromValue)
		selfAuthored := senderOK
		if selfAuthored {
			_, selfAuthored = selfSet[normalizeSuggestionEmail(sender.Address)]
		}
		if selfAuthored {
			for _, addr := range parseAddressListLoose(toValue) {
				accumulateRecipientSuggestion(suggestions, selfSet, addr, 900+recencyWeight, internalDate)
			}
			for _, addr := range parseAddressListLoose(ccValue) {
				accumulateRecipientSuggestion(suggestions, selfSet, addr, 700+recencyWeight, internalDate)
			}
			for _, addr := range parseAddressListLoose(bccValue) {
				accumulateRecipientSuggestion(suggestions, selfSet, addr, 600+recencyWeight, internalDate)
			}
			continue
		}
		if senderOK {
			accumulateRecipientSuggestion(suggestions, selfSet, sender, 300+recencyWeight, internalDate)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	query = strings.ToLower(strings.TrimSpace(query))
	items := make([]recipientScore, 0, len(suggestions))
	for _, item := range suggestions {
		if query != "" && !strings.Contains(strings.ToLower(item.Email), query) && !strings.Contains(strings.ToLower(item.Label), query) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if !items[i].LastAt.Equal(items[j].LastAt) {
			return items[i].LastAt.After(items[j].LastAt)
		}
		return items[i].Email < items[j].Email
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]models.RecipientSuggestion, 0, len(items))
	for _, item := range items {
		out = append(out, item.RecipientSuggestion)
	}
	return out, nil
}

func (s *Store) SetIndexedMessageSeen(ctx context.Context, accountID, id string, seen bool) error {
	return s.updateIndexedMessageBool(ctx, accountID, id, "seen", seen)
}

func (s *Store) SetIndexedMessageFlagged(ctx context.Context, accountID, id string, flagged bool) error {
	return s.updateIndexedMessageBool(ctx, accountID, id, "flagged", flagged)
}

func (s *Store) SetIndexedMessageAnswered(ctx context.Context, accountID, id string, answered bool) error {
	return s.updateIndexedMessageBool(ctx, accountID, id, "answered", answered)
}

func (s *Store) MoveIndexedMessageMailbox(ctx context.Context, accountID, id, mailbox string) error {
	candidates := indexedMessageIDCandidates(accountID, id)
	if len(candidates) == 0 {
		return ErrNotFound
	}
	for _, candidate := range candidates {
		res, err := s.db.ExecContext(ctx,
			`UPDATE message_index SET mailbox=?, updated_at=? WHERE account_id=? AND id=?`,
			mailbox, time.Now().UTC(), accountID, candidate,
		)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			return nil
		}
	}
	return ErrNotFound
}

func (s *Store) DeleteIndexedMessage(ctx context.Context, accountID, id string) error {
	candidates := indexedMessageIDCandidates(accountID, id)
	if len(candidates) == 0 {
		return ErrNotFound
	}
	for _, candidate := range candidates {
		res, err := s.db.ExecContext(ctx, `DELETE FROM message_index WHERE account_id=? AND id=?`, accountID, candidate)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			return nil
		}
	}
	return ErrNotFound
}

func (s *Store) SetIndexedMessageRemoteImagesAllowed(ctx context.Context, accountID, id string, allowed bool) error {
	candidates := indexedMessageIDCandidates(accountID, id)
	if len(candidates) == 0 {
		return ErrNotFound
	}
	for _, candidate := range candidates {
		res, err := s.db.ExecContext(ctx,
			`UPDATE message_index
			 SET remote_images_allowed=?, remote_images_blocked=?, updated_at=?
			 WHERE account_id=? AND id=?`,
			boolToInt(allowed), boolToInt(!allowed), time.Now().UTC(), accountID, candidate,
		)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			return nil
		}
	}
	return ErrNotFound
}

func (s *Store) FindIndexedThreadIDByMessageHeaders(ctx context.Context, accountID string, messageIDs []string) (string, error) {
	normalized := mail.NormalizeMessageIDHeaders(messageIDs)
	if len(normalized) == 0 {
		return "", ErrNotFound
	}
	args := make([]any, 0, len(normalized)+1)
	args = append(args, accountID)
	for _, item := range normalized {
		args = append(args, item)
	}
	row := s.db.QueryRowContext(
		ctx,
		fmt.Sprintf(
			`SELECT thread_id
			 FROM message_index
			 WHERE account_id=? AND message_id_header IN (%s)
			 ORDER BY internal_date ASC
			 LIMIT 1`,
			placeholders(len(normalized)),
		),
		args...,
	)
	var threadID string
	if err := row.Scan(&threadID); err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	return threadID, nil
}

func (s *Store) ListSieveScripts(ctx context.Context, accountID string) ([]models.SieveScript, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,account_id,script_name,script_body,checksum_sha256,is_active,source,created_at,updated_at
		 FROM sieve_cache
		 WHERE account_id=?
		 ORDER BY is_active DESC, script_name ASC`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SieveScript, 0, 8)
	for rows.Next() {
		var item models.SieveScript
		var isActive int
		if err := rows.Scan(
			&item.ID, &item.AccountID, &item.ScriptName, &item.ScriptBody, &item.ChecksumSHA, &isActive, &item.Source, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.IsActive = isActive == 1
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetSieveScript(ctx context.Context, accountID, name string) (models.SieveScript, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,account_id,script_name,script_body,checksum_sha256,is_active,source,created_at,updated_at
		 FROM sieve_cache
		 WHERE account_id=? AND script_name=?`,
		accountID, name,
	)
	var item models.SieveScript
	var isActive int
	if err := row.Scan(
		&item.ID, &item.AccountID, &item.ScriptName, &item.ScriptBody, &item.ChecksumSHA, &isActive, &item.Source, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return models.SieveScript{}, ErrNotFound
		}
		return models.SieveScript{}, err
	}
	item.IsActive = isActive == 1
	return item, nil
}

func (s *Store) UpsertSieveScript(ctx context.Context, in models.SieveScript) (models.SieveScript, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	if strings.TrimSpace(in.Source) == "" {
		in.Source = "cache"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sieve_cache(id,account_id,script_name,script_body,checksum_sha256,is_active,source,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(account_id,script_name) DO UPDATE SET
		   script_body=excluded.script_body,
		   checksum_sha256=excluded.checksum_sha256,
		   source=excluded.source,
		   updated_at=excluded.updated_at`,
		in.ID, in.AccountID, in.ScriptName, in.ScriptBody, in.ChecksumSHA, boolToInt(in.IsActive), in.Source, coalesceTime(in.CreatedAt, now), in.UpdatedAt,
	)
	if err != nil {
		return models.SieveScript{}, err
	}
	return s.GetSieveScript(ctx, in.AccountID, in.ScriptName)
}

func (s *Store) ActivateSieveScript(ctx context.Context, accountID, name string) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sieve_cache SET is_active=CASE WHEN script_name=? THEN 1 ELSE 0 END, updated_at=? WHERE account_id=?`,
		name, time.Now().UTC(), accountID,
	); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE sieve_profiles SET active_script=?, updated_at=? WHERE account_id=?`,
		name, time.Now().UTC(), accountID,
	); err != nil {
		return err
	}
	return nil
}

func (s *Store) DeleteSieveScript(ctx context.Context, accountID, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sieve_cache WHERE account_id=? AND script_name=?`, accountID, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetMFAStatus(ctx context.Context, userID string) (models.MFAStatus, error) {
	out := models.MFAStatus{}
	var hasSecret, enabled int
	_ = s.db.QueryRowContext(ctx, `SELECT CASE WHEN length(trim(secret_enc))>0 THEN 1 ELSE 0 END, enabled FROM mfa_totp WHERE user_id=?`, userID).Scan(&hasSecret, &enabled)
	out.HasTOTP = hasSecret == 1
	out.TOTPEnabled = enabled == 1

	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM mfa_webauthn_credentials WHERE user_id=?`, userID).Scan(&out.WebAuthnCount)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM mfa_recovery_codes WHERE user_id=?`, userID).Scan(&out.RecoveryCodes)
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM mfa_recovery_codes WHERE user_id=? AND used_at IS NULL`, userID).Scan(&out.RecoveryUnused)
	return out, nil
}

func (s *Store) UpsertMFATOTP(ctx context.Context, in models.MFATOTPRecord) (models.MFATOTPRecord, error) {
	now := time.Now().UTC()
	in.UpdatedAt = now
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO mfa_totp(user_id,secret_enc,issuer,account_name,enabled,enrolled_at,updated_at)
		 VALUES(?,?,?,?,?,?,?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   secret_enc=excluded.secret_enc,
		   issuer=excluded.issuer,
		   account_name=excluded.account_name,
		   enabled=excluded.enabled,
		   enrolled_at=excluded.enrolled_at,
		   updated_at=excluded.updated_at`,
		in.UserID,
		in.SecretEnc,
		in.Issuer,
		in.AccountName,
		boolToInt(in.Enabled),
		nullTimeValue(in.EnrolledAt),
		in.UpdatedAt,
	); err != nil {
		return models.MFATOTPRecord{}, err
	}
	return s.GetMFATOTP(ctx, in.UserID)
}

func (s *Store) GetMFATOTP(ctx context.Context, userID string) (models.MFATOTPRecord, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT user_id,secret_enc,issuer,account_name,enabled,enrolled_at,updated_at
		 FROM mfa_totp
		 WHERE user_id=?`,
		userID,
	)
	var item models.MFATOTPRecord
	var enabled int
	var enrolledAt sql.NullTime
	if err := row.Scan(
		&item.UserID, &item.SecretEnc, &item.Issuer, &item.AccountName, &enabled, &enrolledAt, &item.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return models.MFATOTPRecord{}, ErrNotFound
		}
		return models.MFATOTPRecord{}, err
	}
	item.Enabled = enabled == 1
	if enrolledAt.Valid {
		item.EnrolledAt = enrolledAt.Time
	}
	return item, nil
}

func (s *Store) ListMFAWebAuthnCredentials(ctx context.Context, userID string) ([]models.MFAWebAuthnCredential, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,credential_id,public_key,sign_count,transports_json,name,created_at,last_used_at
		 FROM mfa_webauthn_credentials
		 WHERE user_id=?
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.MFAWebAuthnCredential, 0, 4)
	for rows.Next() {
		item, err := scanMFAWebAuthnCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetMFAWebAuthnCredentialByCredentialID(ctx context.Context, userID, credentialID string) (models.MFAWebAuthnCredential, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,credential_id,public_key,sign_count,transports_json,name,created_at,last_used_at
		 FROM mfa_webauthn_credentials
		 WHERE user_id=? AND credential_id=?`,
		userID, credentialID,
	)
	item, err := scanMFAWebAuthnCredential(row)
	if err == sql.ErrNoRows {
		return models.MFAWebAuthnCredential{}, ErrNotFound
	}
	if err != nil {
		return models.MFAWebAuthnCredential{}, err
	}
	return item, nil
}

func (s *Store) GetMFAWebAuthnCredentialByCredentialIDAnyUser(ctx context.Context, credentialID string) (models.MFAWebAuthnCredential, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,credential_id,public_key,sign_count,transports_json,name,created_at,last_used_at
		 FROM mfa_webauthn_credentials
		 WHERE credential_id=?`,
		credentialID,
	)
	item, err := scanMFAWebAuthnCredential(row)
	if err == sql.ErrNoRows {
		return models.MFAWebAuthnCredential{}, ErrNotFound
	}
	if err != nil {
		return models.MFAWebAuthnCredential{}, err
	}
	return item, nil
}

func (s *Store) UpsertMFAWebAuthnCredential(ctx context.Context, in models.MFAWebAuthnCredential) (models.MFAWebAuthnCredential, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	}
	if strings.TrimSpace(in.TransportsJSON) == "" {
		in.TransportsJSON = "[]"
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO mfa_webauthn_credentials(
		 id,user_id,credential_id,public_key,sign_count,transports_json,name,created_at,last_used_at
		) VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(credential_id) DO UPDATE SET
		 public_key=excluded.public_key,
		 sign_count=CASE
		   WHEN excluded.sign_count > mfa_webauthn_credentials.sign_count THEN excluded.sign_count
		   ELSE mfa_webauthn_credentials.sign_count
		 END,
		 transports_json=excluded.transports_json,
		 name=excluded.name,
		 last_used_at=COALESCE(excluded.last_used_at, mfa_webauthn_credentials.last_used_at)`,
		in.ID,
		in.UserID,
		in.CredentialID,
		in.PublicKey,
		in.SignCount,
		in.TransportsJSON,
		in.Name,
		in.CreatedAt,
		nullTimeValue(in.LastUsedAt),
	); err != nil {
		return models.MFAWebAuthnCredential{}, err
	}
	return s.GetMFAWebAuthnCredentialByCredentialID(ctx, in.UserID, in.CredentialID)
}

func (s *Store) TouchMFAWebAuthnCredential(ctx context.Context, userID, id string, signCount int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE mfa_webauthn_credentials
		 SET sign_count=CASE WHEN ? > sign_count THEN ? ELSE sign_count END,
		     last_used_at=?
		 WHERE user_id=? AND id=?`,
		signCount, signCount, time.Now().UTC(), userID, id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteMFAWebAuthnCredential(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mfa_webauthn_credentials WHERE user_id=? AND id=?`, userID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RenameMFAWebAuthnCredential(ctx context.Context, userID, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("credential name is required")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE mfa_webauthn_credentials
		 SET name=?
		 WHERE user_id=? AND id=?`,
		name,
		userID,
		id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ReplaceMFARecoveryCodes(ctx context.Context, userID string, hashes []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM mfa_recovery_codes WHERE user_id=?`, userID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, h := range hashes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO mfa_recovery_codes(id,user_id,code_hash,created_at) VALUES(?,?,?,?)`,
			uuid.NewString(), userID, h, now,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ConsumeRecoveryCodeHash(ctx context.Context, userID, codeHash string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE mfa_recovery_codes SET used_at=? WHERE user_id=? AND code_hash=? AND used_at IS NULL`,
		time.Now().UTC(), userID, codeHash,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) CreateMFATrustedDevice(ctx context.Context, in models.MFATrustedDevice) (models.MFATrustedDevice, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	}
	if in.ExpiresAt.IsZero() {
		in.ExpiresAt = now.Add(30 * 24 * time.Hour)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO mfa_trusted_devices(id,user_id,token_hash,ua_hash,ip_hint,device_label,created_at,last_used_at,expires_at,revoked_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.UserID,
		in.TokenHash,
		in.UAHash,
		in.IPHint,
		in.DeviceLabel,
		in.CreatedAt,
		nullTimeValue(in.LastUsedAt),
		in.ExpiresAt,
		nullTimeValue(in.RevokedAt),
	); err != nil {
		return models.MFATrustedDevice{}, err
	}
	return s.GetMFATrustedDeviceByID(ctx, in.UserID, in.ID)
}

func (s *Store) GetMFATrustedDeviceByID(ctx context.Context, userID, id string) (models.MFATrustedDevice, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,token_hash,ua_hash,ip_hint,device_label,created_at,last_used_at,expires_at,revoked_at
		 FROM mfa_trusted_devices
		 WHERE user_id=? AND id=?`,
		userID,
		id,
	)
	item, err := scanMFATrustedDevice(row)
	if err == sql.ErrNoRows {
		return models.MFATrustedDevice{}, ErrNotFound
	}
	if err != nil {
		return models.MFATrustedDevice{}, err
	}
	return item, nil
}

func (s *Store) GetActiveMFATrustedDeviceByTokenHash(ctx context.Context, userID, tokenHash string, now time.Time) (models.MFATrustedDevice, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,token_hash,ua_hash,ip_hint,device_label,created_at,last_used_at,expires_at,revoked_at
		 FROM mfa_trusted_devices
		 WHERE user_id=? AND token_hash=? AND revoked_at IS NULL AND expires_at>?`,
		userID,
		tokenHash,
		now,
	)
	item, err := scanMFATrustedDevice(row)
	if err == sql.ErrNoRows {
		return models.MFATrustedDevice{}, ErrNotFound
	}
	if err != nil {
		return models.MFATrustedDevice{}, err
	}
	return item, nil
}

func (s *Store) RotateMFATrustedDeviceToken(ctx context.Context, userID, id, tokenHash, uaHash, ipHint string, expiresAt time.Time) (models.MFATrustedDevice, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE mfa_trusted_devices
		 SET token_hash=?,
		     ua_hash=?,
		     ip_hint=?,
		     last_used_at=?,
		     expires_at=?
		 WHERE user_id=? AND id=? AND revoked_at IS NULL`,
		tokenHash,
		uaHash,
		ipHint,
		now,
		expiresAt,
		userID,
		id,
	)
	if err != nil {
		return models.MFATrustedDevice{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.MFATrustedDevice{}, ErrNotFound
	}
	return s.GetMFATrustedDeviceByID(ctx, userID, id)
}

func (s *Store) ListActiveMFATrustedDevices(ctx context.Context, userID string, now time.Time) ([]models.MFATrustedDevice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,token_hash,ua_hash,ip_hint,device_label,created_at,last_used_at,expires_at,revoked_at
		 FROM mfa_trusted_devices
		 WHERE user_id=? AND revoked_at IS NULL AND expires_at>?
		 ORDER BY COALESCE(last_used_at,created_at) DESC`,
		userID,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.MFATrustedDevice, 0, 8)
	for rows.Next() {
		item, err := scanMFATrustedDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) RevokeMFATrustedDevice(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE mfa_trusted_devices SET revoked_at=? WHERE user_id=? AND id=? AND revoked_at IS NULL`,
		time.Now().UTC(),
		userID,
		id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RevokeAllMFATrustedDevices(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE mfa_trusted_devices SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`,
		time.Now().UTC(),
		userID,
	)
	return err
}

func (s *Store) ListSessionsMeta(ctx context.Context, userID string) ([]models.SessionMeta, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id,s.user_id,COALESCE(m.device_label,''),COALESCE(m.ua_summary,''),COALESCE(m.ip_hint,s.ip_hint,''),COALESCE(s.auth_method,'password'),s.mfa_verified_at,s.created_at,s.last_seen_at,s.expires_at,s.idle_expires_at,s.revoked_at,COALESCE(m.revoked_reason,'')
		 FROM sessions s
		 LEFT JOIN user_sessions_meta m ON m.session_id=s.id
		 WHERE s.user_id=?
		 ORDER BY s.last_seen_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.SessionMeta, 0, 8)
	for rows.Next() {
		var item models.SessionMeta
		var mfaAt, revokedAt sql.NullTime
		if err := rows.Scan(
			&item.SessionID,
			&item.UserID,
			&item.DeviceLabel,
			&item.UASummary,
			&item.IPHint,
			&item.AuthMethod,
			&mfaAt,
			&item.CreatedAt,
			&item.LastSeenAt,
			&item.ExpiresAt,
			&item.IdleExpiresAt,
			&revokedAt,
			&item.RevokedReason,
		); err != nil {
			return nil, err
		}
		if mfaAt.Valid {
			item.MFAVerifiedAt = mfaAt.Time
		}
		if revokedAt.Valid {
			item.RevokedAt = revokedAt.Time
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) RevokeSessionWithReason(ctx context.Context, userID, sessionID, reason string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE id=? AND user_id=?`, now, sessionID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO user_sessions_meta(session_id,user_id,device_label,ua_summary,ip_hint,revoked_reason,updated_at)
		 VALUES(?,?,?,?,?,?,?)
		 ON CONFLICT(session_id) DO UPDATE SET revoked_reason=excluded.revoked_reason, updated_at=excluded.updated_at`,
		sessionID, userID, "", "", "", reason, now,
	)
	return nil
}

func (s *Store) GetQuotaCacheByAccount(ctx context.Context, accountID string) (models.QuotaCache, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,account_id,used_bytes,total_bytes,used_messages,total_messages,refreshed_at,COALESCE(last_error,'')
		 FROM quota_cache
		 WHERE account_id=?`,
		accountID,
	)
	var item models.QuotaCache
	if err := row.Scan(
		&item.ID,
		&item.AccountID,
		&item.UsedBytes,
		&item.TotalBytes,
		&item.UsedMessages,
		&item.TotalMessages,
		&item.RefreshedAt,
		&item.LastError,
	); err != nil {
		if err == sql.ErrNoRows {
			return models.QuotaCache{}, ErrNotFound
		}
		return models.QuotaCache{}, err
	}
	return item, nil
}

func (s *Store) UpsertQuotaCache(ctx context.Context, in models.QuotaCache) (models.QuotaCache, error) {
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	if in.RefreshedAt.IsZero() {
		in.RefreshedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO quota_cache(id,account_id,used_bytes,total_bytes,used_messages,total_messages,refreshed_at,last_error)
		 VALUES(?,?,?,?,?,?,?,?)
		 ON CONFLICT(account_id) DO UPDATE SET
		   used_bytes=excluded.used_bytes,
		   total_bytes=excluded.total_bytes,
		   used_messages=excluded.used_messages,
		   total_messages=excluded.total_messages,
		   refreshed_at=excluded.refreshed_at,
		   last_error=excluded.last_error`,
		in.ID, in.AccountID, in.UsedBytes, in.TotalBytes, in.UsedMessages, in.TotalMessages, in.RefreshedAt, in.LastError,
	)
	if err != nil {
		return models.QuotaCache{}, err
	}
	return s.GetQuotaCacheByAccount(ctx, in.AccountID)
}

func (s *Store) ListCryptoKeyrings(ctx context.Context, userID, accountID, kind string) ([]models.CryptoKeyring, error) {
	where := []string{"user_id=?"}
	args := []any{userID}
	if strings.TrimSpace(accountID) != "" {
		where = append(where, "account_id=?")
		args = append(args, accountID)
	}
	if strings.TrimSpace(kind) != "" {
		where = append(where, "kind=?")
		args = append(args, kind)
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(
			`SELECT id,user_id,account_id,kind,fingerprint,user_ids_json,public_key,private_key_enc,passphrase_hint,expires_at,trust_level,created_at,updated_at
			 FROM crypto_keyrings
			 WHERE %s
			 ORDER BY created_at DESC`,
			strings.Join(where, " AND "),
		),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.CryptoKeyring, 0, 8)
	for rows.Next() {
		item, err := scanCryptoKeyring(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetCryptoKeyringByID(ctx context.Context, userID, id string) (models.CryptoKeyring, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,account_id,kind,fingerprint,user_ids_json,public_key,private_key_enc,passphrase_hint,expires_at,trust_level,created_at,updated_at
		 FROM crypto_keyrings
		 WHERE user_id=? AND id=?`,
		userID, id,
	)
	item, err := scanCryptoKeyring(row)
	if err == sql.ErrNoRows {
		return models.CryptoKeyring{}, ErrNotFound
	}
	if err != nil {
		return models.CryptoKeyring{}, err
	}
	return item, nil
}

func (s *Store) CreateCryptoKeyring(ctx context.Context, in models.CryptoKeyring) (models.CryptoKeyring, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	if strings.TrimSpace(in.UserIDsJSON) == "" {
		in.UserIDsJSON = "[]"
	}
	if strings.TrimSpace(in.TrustLevel) == "" {
		in.TrustLevel = "unknown"
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO crypto_keyrings(
		  id,user_id,account_id,kind,fingerprint,user_ids_json,public_key,private_key_enc,passphrase_hint,expires_at,trust_level,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.UserID,
		in.AccountID,
		in.Kind,
		in.Fingerprint,
		in.UserIDsJSON,
		in.PublicKey,
		in.PrivateKeyEnc,
		in.PassphraseHint,
		nullTimeValue(in.ExpiresAt),
		in.TrustLevel,
		in.CreatedAt,
		in.UpdatedAt,
	)
	if err != nil {
		return models.CryptoKeyring{}, err
	}
	return s.GetCryptoKeyringByID(ctx, in.UserID, in.ID)
}

func (s *Store) UpdateCryptoKeyring(ctx context.Context, in models.CryptoKeyring) (models.CryptoKeyring, error) {
	in.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE crypto_keyrings
		 SET account_id=?,kind=?,fingerprint=?,user_ids_json=?,public_key=?,private_key_enc=?,passphrase_hint=?,expires_at=?,trust_level=?,updated_at=?
		 WHERE user_id=? AND id=?`,
		in.AccountID,
		in.Kind,
		in.Fingerprint,
		in.UserIDsJSON,
		in.PublicKey,
		in.PrivateKeyEnc,
		in.PassphraseHint,
		nullTimeValue(in.ExpiresAt),
		in.TrustLevel,
		in.UpdatedAt,
		in.UserID,
		in.ID,
	)
	if err != nil {
		return models.CryptoKeyring{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.CryptoKeyring{}, ErrNotFound
	}
	return s.GetCryptoKeyringByID(ctx, in.UserID, in.ID)
}

func (s *Store) DeleteCryptoKeyring(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM crypto_keyrings WHERE user_id=? AND id=?`, userID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListCryptoTrustPolicies(ctx context.Context, userID, accountID string) ([]models.CryptoTrustPolicy, error) {
	where := []string{"user_id=?"}
	args := []any{userID}
	if strings.TrimSpace(accountID) != "" {
		where = append(where, "account_id=?")
		args = append(args, accountID)
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(
			`SELECT id,user_id,account_id,sender_pattern,domain_pattern,min_trust_level,require_signed,require_encrypted,created_at,updated_at
			 FROM crypto_trust_policies
			 WHERE %s
			 ORDER BY created_at DESC`,
			strings.Join(where, " AND "),
		),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.CryptoTrustPolicy, 0, 8)
	for rows.Next() {
		item, err := scanCryptoTrustPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetCryptoTrustPolicyByID(ctx context.Context, userID, id string) (models.CryptoTrustPolicy, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,account_id,sender_pattern,domain_pattern,min_trust_level,require_signed,require_encrypted,created_at,updated_at
		 FROM crypto_trust_policies
		 WHERE user_id=? AND id=?`,
		userID, id,
	)
	item, err := scanCryptoTrustPolicy(row)
	if err == sql.ErrNoRows {
		return models.CryptoTrustPolicy{}, ErrNotFound
	}
	if err != nil {
		return models.CryptoTrustPolicy{}, err
	}
	return item, nil
}

func (s *Store) CreateCryptoTrustPolicy(ctx context.Context, in models.CryptoTrustPolicy) (models.CryptoTrustPolicy, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	if strings.TrimSpace(in.MinTrustLevel) == "" {
		in.MinTrustLevel = "unknown"
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO crypto_trust_policies(
		  id,user_id,account_id,sender_pattern,domain_pattern,min_trust_level,require_signed,require_encrypted,created_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.UserID,
		in.AccountID,
		in.SenderPattern,
		in.DomainPattern,
		in.MinTrustLevel,
		boolToInt(in.RequireSigned),
		boolToInt(in.RequireEncrypted),
		in.CreatedAt,
		in.UpdatedAt,
	)
	if err != nil {
		return models.CryptoTrustPolicy{}, err
	}
	return s.GetCryptoTrustPolicyByID(ctx, in.UserID, in.ID)
}

func (s *Store) UpdateCryptoTrustPolicy(ctx context.Context, in models.CryptoTrustPolicy) (models.CryptoTrustPolicy, error) {
	in.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE crypto_trust_policies
		 SET account_id=?,sender_pattern=?,domain_pattern=?,min_trust_level=?,require_signed=?,require_encrypted=?,updated_at=?
		 WHERE user_id=? AND id=?`,
		in.AccountID,
		in.SenderPattern,
		in.DomainPattern,
		in.MinTrustLevel,
		boolToInt(in.RequireSigned),
		boolToInt(in.RequireEncrypted),
		in.UpdatedAt,
		in.UserID,
		in.ID,
	)
	if err != nil {
		return models.CryptoTrustPolicy{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return models.CryptoTrustPolicy{}, ErrNotFound
	}
	return s.GetCryptoTrustPolicyByID(ctx, in.UserID, in.ID)
}

func (s *Store) DeleteCryptoTrustPolicy(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM crypto_trust_policies WHERE user_id=? AND id=?`, userID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) AddRemoteImageAllowlist(ctx context.Context, userID, messageID, sender string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO remote_image_allowlist(id,user_id,message_id,sender,created_at) VALUES(?,?,?,?,?)`,
		uuid.NewString(), userID, messageID, strings.ToLower(strings.TrimSpace(sender)), time.Now().UTC(),
	)
	return err
}

func (s *Store) updateIndexedMessageBool(ctx context.Context, accountID, id, column string, value bool) error {
	if column != "seen" && column != "flagged" && column != "answered" {
		return fmt.Errorf("unsupported message boolean column")
	}
	candidates := indexedMessageIDCandidates(accountID, id)
	if len(candidates) == 0 {
		return ErrNotFound
	}
	query := fmt.Sprintf(`UPDATE message_index SET %s=?, updated_at=? WHERE account_id=? AND id=?`, column)
	for _, candidate := range candidates {
		res, err := s.db.ExecContext(ctx, query, boolToInt(value), time.Now().UTC(), accountID, candidate)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			return nil
		}
	}
	return ErrNotFound
}

func normalizeSuggestionEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func firstLooseAddress(raw string) (stdmail.Address, bool) {
	items := parseAddressListLoose(raw)
	if len(items) == 0 {
		return stdmail.Address{}, false
	}
	return items[0], true
}

func parseAddressListLoose(raw string) []stdmail.Address {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if parsed, err := stdmail.ParseAddressList(raw); err == nil {
		items := make([]stdmail.Address, 0, len(parsed))
		for _, item := range parsed {
			if item == nil {
				continue
			}
			items = append(items, *item)
		}
		return dedupeLooseAddresses(items)
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	out := make([]stdmail.Address, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		addr, err := stdmail.ParseAddress(part)
		if err == nil {
			out = append(out, *addr)
			continue
		}
		email := strings.Trim(strings.TrimSpace(part), "<>")
		if !strings.Contains(email, "@") {
			continue
		}
		out = append(out, stdmail.Address{Address: email})
	}
	return dedupeLooseAddresses(out)
}

func dedupeLooseAddresses(items []stdmail.Address) []stdmail.Address {
	seen := map[string]struct{}{}
	out := make([]stdmail.Address, 0, len(items))
	for _, item := range items {
		email := normalizeSuggestionEmail(item.Address)
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		item.Address = email
		out = append(out, item)
	}
	return out
}

func accumulateRecipientSuggestion(items map[string]recipientScore, selfSet map[string]struct{}, addr stdmail.Address, score int, lastAt time.Time) {
	email := normalizeSuggestionEmail(addr.Address)
	if email == "" {
		return
	}
	if _, ok := selfSet[email]; ok {
		return
	}
	current := items[email]
	current.Email = email
	current.Score += score
	if current.Label == "" || (current.Label == current.Email && strings.TrimSpace(addr.Name) != "") {
		if strings.TrimSpace(addr.Name) != "" {
			current.Label = (&stdmail.Address{Name: strings.TrimSpace(addr.Name), Address: email}).String()
		} else {
			current.Label = email
		}
	}
	if current.LastAt.IsZero() || lastAt.After(current.LastAt) {
		current.LastAt = lastAt
	}
	items[email] = current
}

func scanMailAccount(scanner interface{ Scan(dest ...any) error }) (models.MailAccount, error) {
	var item models.MailAccount
	var imapTLS, imapStartTLS, smtpTLS, smtpStartTLS, isDefault int
	var lastSyncAt sql.NullTime
	var lastError sql.NullString
	err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.DisplayName,
		&item.Login,
		&item.SecretEnc,
		&item.IMAPHost,
		&item.IMAPPort,
		&imapTLS,
		&imapStartTLS,
		&item.SMTPHost,
		&item.SMTPPort,
		&smtpTLS,
		&smtpStartTLS,
		&isDefault,
		&item.Status,
		&lastSyncAt,
		&lastError,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return models.MailAccount{}, err
	}
	item.IMAPTLS = imapTLS == 1
	item.IMAPStartTLS = imapStartTLS == 1
	item.SMTPTLS = smtpTLS == 1
	item.SMTPStartTLS = smtpStartTLS == 1
	item.IsDefault = isDefault == 1
	if lastSyncAt.Valid {
		item.LastSyncAt = lastSyncAt.Time
	}
	if lastError.Valid {
		item.LastError = lastError.String
	}
	return item, nil
}

func scanCryptoKeyring(scanner interface{ Scan(dest ...any) error }) (models.CryptoKeyring, error) {
	var item models.CryptoKeyring
	var expiresAt sql.NullTime
	if err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.AccountID,
		&item.Kind,
		&item.Fingerprint,
		&item.UserIDsJSON,
		&item.PublicKey,
		&item.PrivateKeyEnc,
		&item.PassphraseHint,
		&expiresAt,
		&item.TrustLevel,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.CryptoKeyring{}, err
	}
	if expiresAt.Valid {
		item.ExpiresAt = expiresAt.Time
	}
	return item, nil
}

func scanCryptoTrustPolicy(scanner interface{ Scan(dest ...any) error }) (models.CryptoTrustPolicy, error) {
	var item models.CryptoTrustPolicy
	var requireSigned, requireEncrypted int
	if err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.AccountID,
		&item.SenderPattern,
		&item.DomainPattern,
		&item.MinTrustLevel,
		&requireSigned,
		&requireEncrypted,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.CryptoTrustPolicy{}, err
	}
	item.RequireSigned = requireSigned == 1
	item.RequireEncrypted = requireEncrypted == 1
	return item, nil
}

func scanMFAWebAuthnCredential(scanner interface{ Scan(dest ...any) error }) (models.MFAWebAuthnCredential, error) {
	var item models.MFAWebAuthnCredential
	var lastUsedAt sql.NullTime
	err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.CredentialID,
		&item.PublicKey,
		&item.SignCount,
		&item.TransportsJSON,
		&item.Name,
		&item.CreatedAt,
		&lastUsedAt,
	)
	if err != nil {
		return models.MFAWebAuthnCredential{}, err
	}
	if lastUsedAt.Valid {
		item.LastUsedAt = lastUsedAt.Time
	}
	return item, nil
}

func scanMFATrustedDevice(scanner interface{ Scan(dest ...any) error }) (models.MFATrustedDevice, error) {
	var item models.MFATrustedDevice
	var lastUsedAt, revokedAt sql.NullTime
	err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.TokenHash,
		&item.UAHash,
		&item.IPHint,
		&item.DeviceLabel,
		&item.CreatedAt,
		&lastUsedAt,
		&item.ExpiresAt,
		&revokedAt,
	)
	if err != nil {
		return models.MFATrustedDevice{}, err
	}
	if lastUsedAt.Valid {
		item.LastUsedAt = lastUsedAt.Time
	}
	if revokedAt.Valid {
		item.RevokedAt = revokedAt.Time
	}
	return item, nil
}

func scanIndexedMessage(scanner interface{ Scan(dest ...any) error }) (models.IndexedMessage, error) {
	var item models.IndexedMessage
	var seen, flagged, answered, draft, hasAttachments int
	var remoteBlocked, remoteAllowed int
	if err := scanner.Scan(
		&item.ID,
		&item.AccountID,
		&item.Mailbox,
		&item.UID,
		&item.ThreadID,
		&item.MessageIDHeader,
		&item.InReplyToHeader,
		&item.ReferencesHeader,
		&item.FromValue,
		&item.ToValue,
		&item.CCValue,
		&item.BCCValue,
		&item.Subject,
		&item.Snippet,
		&item.BodyText,
		&item.BodyHTMLSanitized,
		&item.RawSource,
		&seen,
		&flagged,
		&answered,
		&draft,
		&hasAttachments,
		&item.Importance,
		&item.DKIMStatus,
		&item.SPFStatus,
		&item.DMARCStatus,
		&item.PhishingScore,
		&remoteBlocked,
		&remoteAllowed,
		&item.DateHeader,
		&item.InternalDate,
	); err != nil {
		return models.IndexedMessage{}, err
	}
	item.Seen = seen == 1
	item.Flagged = flagged == 1
	item.Answered = answered == 1
	item.Draft = draft == 1
	item.HasAttachments = hasAttachments == 1
	item.RemoteImagesBlocked = remoteBlocked == 1
	item.RemoteImagesAllowed = remoteAllowed == 1
	return item, nil
}

func scanDraft(scanner interface{ Scan(dest ...any) error }) (models.Draft, error) {
	var item models.Draft
	var accountID sql.NullString
	var scheduledAt sql.NullTime
	if err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&accountID,
		&item.IdentityID,
		&item.ComposeMode,
		&item.ContextMessageID,
		&item.ContextAccountID,
		&item.FromMode,
		&item.FromManual,
		&item.ClientStateJSON,
		&item.ToValue,
		&item.CCValue,
		&item.BCCValue,
		&item.Subject,
		&item.BodyText,
		&item.BodyHTML,
		&item.AttachmentsJSON,
		&item.CryptoOptions,
		&item.SendMode,
		&scheduledAt,
		&item.Status,
		&item.LastSendError,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.Draft{}, err
	}
	if accountID.Valid {
		item.AccountID = accountID.String
	}
	if scheduledAt.Valid {
		item.ScheduledFor = scheduledAt.Time
	}
	return item, nil
}

func scanDraftAttachment(scanner interface{ Scan(dest ...any) error }) (models.DraftAttachment, error) {
	var item models.DraftAttachment
	var inlinePart int
	if err := scanner.Scan(
		&item.ID,
		&item.DraftID,
		&item.UserID,
		&item.Filename,
		&item.ContentType,
		&item.SizeBytes,
		&inlinePart,
		&item.ContentID,
		&item.SortOrder,
		&item.Data,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.DraftAttachment{}, err
	}
	item.InlinePart = inlinePart == 1
	return item, nil
}

func nullTimeValue(v time.Time) any {
	if v.IsZero() {
		return nil
	}
	return v
}

func nullStringValue(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func normalizeDraftAttachmentsJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "[]"
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return "[]"
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func coalesceTime(v, fallback time.Time) time.Time {
	if v.IsZero() {
		return fallback
	}
	return v
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
