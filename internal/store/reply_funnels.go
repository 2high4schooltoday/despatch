package store

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"

	"despatch/internal/models"
)

type replyFunnelScanner interface {
	Scan(dest ...any) error
}

func nullableTrimmedString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func normalizeReplyFunnelReplyMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "source":
		return "source"
	case "smart":
		return "smart"
	default:
		return "collector"
	}
}

func normalizeReplyFunnelRoutingMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "managed_rules":
		return "managed_rules"
	case "assisted_forwarding":
		return "assisted_forwarding"
	default:
		return "virtual_inbox"
	}
}

func normalizeAssistedForwardingState(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending":
		return "pending"
	case "in_progress":
		return "in_progress"
	case "confirmed":
		return "confirmed"
	case "needs_help":
		return "needs_help"
	default:
		return "not_required"
	}
}

func scanReplyFunnel(row replyFunnelScanner) (models.ReplyFunnel, error) {
	var item models.ReplyFunnel
	var includeCollector int
	var savedSearchID sql.NullString
	if err := row.Scan(
		&item.ID,
		&item.UserID,
		&item.Name,
		&item.SenderName,
		&item.CollectorAccountID,
		&item.ReplyMode,
		&item.RoutingMode,
		&includeCollector,
		&item.TargetReplyCount,
		&savedSearchID,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.ReplyFunnel{}, err
	}
	item.ReplyMode = normalizeReplyFunnelReplyMode(item.ReplyMode)
	item.RoutingMode = normalizeReplyFunnelRoutingMode(item.RoutingMode)
	item.IncludeCollector = includeCollector == 1
	item.SavedSearchID = strings.TrimSpace(savedSearchID.String)
	if item.TargetReplyCount <= 0 {
		item.TargetReplyCount = 100
	}
	return item, nil
}

func (s *Store) ListReplyFunnels(ctx context.Context, userID string) ([]models.ReplyFunnel, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,name,sender_name,collector_account_id,reply_mode,routing_mode,include_collector,target_reply_count,saved_search_id,created_at,updated_at
		 FROM reply_funnels
		 WHERE user_id=?
		 ORDER BY updated_at DESC, name ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ReplyFunnel, 0, 8)
	for rows.Next() {
		item, err := scanReplyFunnel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetReplyFunnelByID(ctx context.Context, userID, id string) (models.ReplyFunnel, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,name,sender_name,collector_account_id,reply_mode,routing_mode,include_collector,target_reply_count,saved_search_id,created_at,updated_at
		 FROM reply_funnels
		 WHERE user_id=? AND id=?`,
		userID, strings.TrimSpace(id),
	)
	item, err := scanReplyFunnel(row)
	if err == sql.ErrNoRows {
		return models.ReplyFunnel{}, ErrNotFound
	}
	if err != nil {
		return models.ReplyFunnel{}, err
	}
	return item, nil
}

func (s *Store) CreateReplyFunnel(ctx context.Context, in models.ReplyFunnel) (models.ReplyFunnel, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.Name = strings.TrimSpace(in.Name)
	in.SenderName = strings.TrimSpace(in.SenderName)
	in.CollectorAccountID = strings.TrimSpace(in.CollectorAccountID)
	in.ReplyMode = normalizeReplyFunnelReplyMode(in.ReplyMode)
	in.RoutingMode = normalizeReplyFunnelRoutingMode(in.RoutingMode)
	if in.TargetReplyCount <= 0 {
		in.TargetReplyCount = 100
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO reply_funnels(id,user_id,name,sender_name,collector_account_id,reply_mode,routing_mode,include_collector,target_reply_count,saved_search_id,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.UserID,
		in.Name,
		in.SenderName,
		in.CollectorAccountID,
		in.ReplyMode,
		in.RoutingMode,
		boolToInt(in.IncludeCollector),
		in.TargetReplyCount,
		nullableTrimmedString(in.SavedSearchID),
		in.CreatedAt,
		in.UpdatedAt,
	)
	if err != nil {
		return models.ReplyFunnel{}, err
	}
	return s.GetReplyFunnelByID(ctx, in.UserID, in.ID)
}

func (s *Store) UpdateReplyFunnel(ctx context.Context, in models.ReplyFunnel) (models.ReplyFunnel, error) {
	now := time.Now().UTC()
	in.Name = strings.TrimSpace(in.Name)
	in.SenderName = strings.TrimSpace(in.SenderName)
	in.CollectorAccountID = strings.TrimSpace(in.CollectorAccountID)
	in.ReplyMode = normalizeReplyFunnelReplyMode(in.ReplyMode)
	in.RoutingMode = normalizeReplyFunnelRoutingMode(in.RoutingMode)
	if in.TargetReplyCount <= 0 {
		in.TargetReplyCount = 100
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE reply_funnels
		 SET name=?, sender_name=?, collector_account_id=?, reply_mode=?, routing_mode=?, include_collector=?, target_reply_count=?, saved_search_id=?, updated_at=?
		 WHERE user_id=? AND id=?`,
		in.Name,
		in.SenderName,
		in.CollectorAccountID,
		in.ReplyMode,
		in.RoutingMode,
		boolToInt(in.IncludeCollector),
		in.TargetReplyCount,
		nullableTrimmedString(in.SavedSearchID),
		now,
		in.UserID,
		in.ID,
	)
	if err != nil {
		return models.ReplyFunnel{}, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return models.ReplyFunnel{}, ErrNotFound
	}
	return s.GetReplyFunnelByID(ctx, in.UserID, in.ID)
}

func (s *Store) DeleteReplyFunnel(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM reply_funnels WHERE user_id=? AND id=?`, userID, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

type replyFunnelAccountScanner interface {
	Scan(dest ...any) error
}

func scanReplyFunnelAccount(row replyFunnelAccountScanner) (models.ReplyFunnelAccount, error) {
	var item models.ReplyFunnelAccount
	var senderIdentityID sql.NullString
	var redirectRuleID sql.NullString
	var assistedCheckedAt sql.NullTime
	var assistedConfirmedAt sql.NullTime
	if err := row.Scan(
		&item.ID,
		&item.FunnelID,
		&item.AccountID,
		&item.Role,
		&item.Position,
		&senderIdentityID,
		&redirectRuleID,
		&item.LastApplyError,
		&item.AssistedForwardingState,
		&item.AssistedForwardingNotes,
		&assistedCheckedAt,
		&assistedConfirmedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.ReplyFunnelAccount{}, err
	}
	item.Role = strings.ToLower(strings.TrimSpace(item.Role))
	item.SenderIdentityID = strings.TrimSpace(senderIdentityID.String)
	item.RedirectRuleID = strings.TrimSpace(redirectRuleID.String)
	item.AssistedForwardingState = normalizeAssistedForwardingState(item.AssistedForwardingState)
	item.AssistedForwardingNotes = strings.TrimSpace(item.AssistedForwardingNotes)
	if assistedCheckedAt.Valid {
		item.AssistedForwardingCheckedAt = assistedCheckedAt.Time
	}
	if assistedConfirmedAt.Valid {
		item.AssistedForwardingConfirmed = assistedConfirmedAt.Time
	}
	return item, nil
}

func (s *Store) ListReplyFunnelAccounts(ctx context.Context, funnelID string) ([]models.ReplyFunnelAccount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,funnel_id,account_id,role,position,sender_identity_id,redirect_rule_id,last_apply_error,assisted_forwarding_state,assisted_forwarding_notes,assisted_forwarding_checked_at,assisted_forwarding_confirmed_at,created_at,updated_at
		 FROM reply_funnel_accounts
		 WHERE funnel_id=?
		 ORDER BY CASE WHEN role='collector' THEN 0 ELSE 1 END, position ASC, created_at ASC`,
		strings.TrimSpace(funnelID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.ReplyFunnelAccount, 0, 16)
	for rows.Next() {
		item, err := scanReplyFunnelAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetReplyFunnelAccountByKey(ctx context.Context, funnelID, accountID, role string) (models.ReplyFunnelAccount, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,funnel_id,account_id,role,position,sender_identity_id,redirect_rule_id,last_apply_error,assisted_forwarding_state,assisted_forwarding_notes,assisted_forwarding_checked_at,assisted_forwarding_confirmed_at,created_at,updated_at
		 FROM reply_funnel_accounts
		 WHERE funnel_id=? AND account_id=? AND role=?`,
		strings.TrimSpace(funnelID),
		strings.TrimSpace(accountID),
		strings.ToLower(strings.TrimSpace(role)),
	)
	item, err := scanReplyFunnelAccount(row)
	if err == sql.ErrNoRows {
		return models.ReplyFunnelAccount{}, ErrNotFound
	}
	if err != nil {
		return models.ReplyFunnelAccount{}, err
	}
	return item, nil
}

func (s *Store) UpsertReplyFunnelAccount(ctx context.Context, in models.ReplyFunnelAccount) (models.ReplyFunnelAccount, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	in.FunnelID = strings.TrimSpace(in.FunnelID)
	in.AccountID = strings.TrimSpace(in.AccountID)
	in.Role = strings.ToLower(strings.TrimSpace(in.Role))
	in.AssistedForwardingState = normalizeAssistedForwardingState(in.AssistedForwardingState)
	in.AssistedForwardingNotes = strings.TrimSpace(in.AssistedForwardingNotes)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO reply_funnel_accounts(id,funnel_id,account_id,role,position,sender_identity_id,redirect_rule_id,last_apply_error,assisted_forwarding_state,assisted_forwarding_notes,assisted_forwarding_checked_at,assisted_forwarding_confirmed_at,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(funnel_id,account_id,role) DO UPDATE SET
		   position=excluded.position,
		   sender_identity_id=excluded.sender_identity_id,
		   redirect_rule_id=excluded.redirect_rule_id,
		   last_apply_error=excluded.last_apply_error,
		   assisted_forwarding_state=excluded.assisted_forwarding_state,
		   assisted_forwarding_notes=excluded.assisted_forwarding_notes,
		   assisted_forwarding_checked_at=excluded.assisted_forwarding_checked_at,
		   assisted_forwarding_confirmed_at=excluded.assisted_forwarding_confirmed_at,
		   updated_at=excluded.updated_at`,
		in.ID,
		in.FunnelID,
		in.AccountID,
		in.Role,
		in.Position,
		nullableTrimmedString(in.SenderIdentityID),
		nullableTrimmedString(in.RedirectRuleID),
		strings.TrimSpace(in.LastApplyError),
		in.AssistedForwardingState,
		in.AssistedForwardingNotes,
		nullTimeValue(in.AssistedForwardingCheckedAt),
		nullTimeValue(in.AssistedForwardingConfirmed),
		coalesceTime(in.CreatedAt, now),
		in.UpdatedAt,
	)
	if err != nil {
		return models.ReplyFunnelAccount{}, err
	}
	return s.GetReplyFunnelAccountByKey(ctx, in.FunnelID, in.AccountID, in.Role)
}

func (s *Store) DeleteReplyFunnelAccountByID(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM reply_funnel_accounts WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
