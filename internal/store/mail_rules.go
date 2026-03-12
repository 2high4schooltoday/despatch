package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"despatch/internal/models"
)

func normalizeMailRuleMatchMode(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "any") {
		return "any"
	}
	return "all"
}

func marshalMailRuleJSON[T any](value T) string {
	b, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func unmarshalMailRuleConditions(raw string) models.MailRuleConditions {
	var out models.MailRuleConditions
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &out)
	return out
}

func unmarshalMailRuleActions(raw string) models.MailRuleActions {
	var out models.MailRuleActions
	_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &out)
	return out
}

type mailRuleScanner interface {
	Scan(dest ...any) error
}

func scanMailRule(row mailRuleScanner) (models.MailRule, error) {
	var item models.MailRule
	var enabled int
	var conditionsJSON string
	var actionsJSON string
	if err := row.Scan(
		&item.ID,
		&item.AccountID,
		&item.Name,
		&enabled,
		&item.Position,
		&item.MatchMode,
		&conditionsJSON,
		&actionsJSON,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return models.MailRule{}, err
	}
	item.Enabled = enabled == 1
	item.MatchMode = normalizeMailRuleMatchMode(item.MatchMode)
	item.Conditions = unmarshalMailRuleConditions(conditionsJSON)
	item.Actions = unmarshalMailRuleActions(actionsJSON)
	return item, nil
}

func (s *Store) ListMailRules(ctx context.Context, accountID string) ([]models.MailRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,account_id,name,enabled,position,match_mode,conditions_json,actions_json,created_at,updated_at
		 FROM mail_rules
		 WHERE account_id=?
		 ORDER BY position ASC, created_at ASC`,
		accountID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.MailRule, 0, 8)
	for rows.Next() {
		item, err := scanMailRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetMailRuleByID(ctx context.Context, accountID, id string) (models.MailRule, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,account_id,name,enabled,position,match_mode,conditions_json,actions_json,created_at,updated_at
		 FROM mail_rules
		 WHERE account_id=? AND id=?`,
		accountID, id,
	)
	item, err := scanMailRule(row)
	if err == sql.ErrNoRows {
		return models.MailRule{}, ErrNotFound
	}
	if err != nil {
		return models.MailRule{}, err
	}
	return item, nil
}

func (s *Store) nextMailRulePosition(ctx context.Context, accountID string) (int, error) {
	var next int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), -1) + 1 FROM mail_rules WHERE account_id=?`, accountID).Scan(&next); err != nil {
		return 0, err
	}
	return next, nil
}

func (s *Store) CreateMailRule(ctx context.Context, in models.MailRule) (models.MailRule, error) {
	now := time.Now().UTC()
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	in.AccountID = strings.TrimSpace(in.AccountID)
	in.Name = strings.TrimSpace(in.Name)
	in.MatchMode = normalizeMailRuleMatchMode(in.MatchMode)
	if in.Position < 0 {
		in.Position = 0
	}
	nextPos, err := s.nextMailRulePosition(ctx, in.AccountID)
	if err != nil {
		return models.MailRule{}, err
	}
	if in.Position == 0 && nextPos > 0 {
		in.Position = nextPos
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO mail_rules(id,account_id,name,enabled,position,match_mode,conditions_json,actions_json,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		in.ID,
		in.AccountID,
		in.Name,
		boolToInt(in.Enabled),
		in.Position,
		in.MatchMode,
		marshalMailRuleJSON(in.Conditions),
		marshalMailRuleJSON(in.Actions),
		in.CreatedAt,
		in.UpdatedAt,
	)
	if err != nil {
		return models.MailRule{}, err
	}
	return s.GetMailRuleByID(ctx, in.AccountID, in.ID)
}

func (s *Store) UpdateMailRule(ctx context.Context, in models.MailRule) (models.MailRule, error) {
	now := time.Now().UTC()
	in.AccountID = strings.TrimSpace(in.AccountID)
	in.Name = strings.TrimSpace(in.Name)
	in.MatchMode = normalizeMailRuleMatchMode(in.MatchMode)
	res, err := s.db.ExecContext(ctx,
		`UPDATE mail_rules
		 SET name=?, enabled=?, position=?, match_mode=?, conditions_json=?, actions_json=?, updated_at=?
		 WHERE account_id=? AND id=?`,
		in.Name,
		boolToInt(in.Enabled),
		in.Position,
		in.MatchMode,
		marshalMailRuleJSON(in.Conditions),
		marshalMailRuleJSON(in.Actions),
		now,
		in.AccountID,
		in.ID,
	)
	if err != nil {
		return models.MailRule{}, err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return models.MailRule{}, ErrNotFound
	}
	return s.GetMailRuleByID(ctx, in.AccountID, in.ID)
}

func (s *Store) DeleteMailRule(ctx context.Context, accountID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mail_rules WHERE account_id=? AND id=?`, accountID, id)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ReorderMailRules(ctx context.Context, accountID string, ids []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		`SELECT id
		 FROM mail_rules
		 WHERE account_id=?
		 ORDER BY position ASC, created_at ASC`,
		accountID,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	existing := make([]string, 0, 8)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		existing = append(existing, strings.TrimSpace(id))
	}
	if err := rows.Err(); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	order := make([]string, 0, len(existing))
	for _, id := range ids {
		key := strings.TrimSpace(id)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		order = append(order, key)
	}
	for _, id := range existing {
		if _, ok := seen[id]; ok {
			continue
		}
		order = append(order, id)
	}
	now := time.Now().UTC()
	for idx, id := range order {
		if _, err := tx.ExecContext(ctx,
			`UPDATE mail_rules SET position=?, updated_at=? WHERE account_id=? AND id=?`,
			idx,
			now,
			accountID,
			id,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ActiveSieveScriptName(ctx context.Context, accountID string) (string, error) {
	var name string
	if err := s.db.QueryRowContext(ctx, `SELECT active_script FROM sieve_profiles WHERE account_id=?`, accountID).Scan(&name); err != nil {
		if err != sql.ErrNoRows {
			return "", err
		}
		name = ""
	}
	name = strings.TrimSpace(name)
	if name != "" {
		return name, nil
	}
	_ = s.db.QueryRowContext(ctx,
		`SELECT script_name FROM sieve_cache WHERE account_id=? AND is_active=1 ORDER BY updated_at DESC LIMIT 1`,
		accountID,
	).Scan(&name)
	return strings.TrimSpace(name), nil
}
