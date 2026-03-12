package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"despatch/internal/models"
)

func isUniqueConstraintErr(err error) bool {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "unique constraint") || strings.Contains(msg, "constraint failed")
}

func dedupeOrderedStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeContactEmails(items []models.ContactEmail) []models.ContactEmail {
	if len(items) == 0 {
		return nil
	}
	out := make([]models.ContactEmail, 0, len(items))
	seen := map[string]struct{}{}
	primarySeen := false
	for _, item := range items {
		email := strings.ToLower(strings.TrimSpace(item.Email))
		if email == "" {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		item.Email = email
		item.Label = strings.TrimSpace(item.Label)
		item.IsPrimary = item.IsPrimary && !primarySeen
		if item.IsPrimary {
			primarySeen = true
		}
		out = append(out, item)
	}
	if len(out) > 0 && !primarySeen {
		out[0].IsPrimary = true
	}
	return out
}

func normalizeContactStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func (s *Store) ListContacts(ctx context.Context, userID, query string) ([]models.Contact, error) {
	ids, err := s.listContactIDs(ctx, userID, query)
	if err != nil {
		return nil, err
	}
	return s.loadContactsByIDs(ctx, userID, ids)
}

func (s *Store) GetContactByID(ctx context.Context, userID, id string) (models.Contact, error) {
	items, err := s.loadContactsByIDs(ctx, userID, []string{strings.TrimSpace(id)})
	if err != nil {
		return models.Contact{}, err
	}
	if len(items) == 0 {
		return models.Contact{}, ErrNotFound
	}
	return items[0], nil
}

func (s *Store) CreateContact(ctx context.Context, in models.Contact) (models.Contact, error) {
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		in.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	in.CreatedAt = now
	in.UpdatedAt = now
	in.Name = strings.TrimSpace(in.Name)
	in.Notes = strings.TrimSpace(in.Notes)
	in.PreferredAccountID = strings.TrimSpace(in.PreferredAccountID)
	in.PreferredSenderID = strings.TrimSpace(in.PreferredSenderID)
	in.Nicknames = normalizeContactStrings(in.Nicknames)
	in.GroupIDs = normalizeContactStrings(in.GroupIDs)
	in.Emails = normalizeContactEmails(in.Emails)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Contact{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO contacts(id,user_id,name,notes,preferred_account_id,preferred_sender_id,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		in.ID, in.UserID, in.Name, in.Notes, in.PreferredAccountID, in.PreferredSenderID, in.CreatedAt, in.UpdatedAt,
	); err != nil {
		if isUniqueConstraintErr(err) {
			return models.Contact{}, ErrConflict
		}
		return models.Contact{}, err
	}
	if err := saveContactDetailsTx(ctx, tx, in); err != nil {
		return models.Contact{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.Contact{}, err
	}
	return s.GetContactByID(ctx, in.UserID, in.ID)
}

func (s *Store) UpdateContact(ctx context.Context, in models.Contact) (models.Contact, error) {
	current, err := s.GetContactByID(ctx, in.UserID, in.ID)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return models.Contact{}, ErrConflict
		}
		return models.Contact{}, err
	}
	now := time.Now().UTC()
	current.Name = strings.TrimSpace(in.Name)
	current.Notes = strings.TrimSpace(in.Notes)
	current.PreferredAccountID = strings.TrimSpace(in.PreferredAccountID)
	current.PreferredSenderID = strings.TrimSpace(in.PreferredSenderID)
	current.Nicknames = normalizeContactStrings(in.Nicknames)
	current.GroupIDs = normalizeContactStrings(in.GroupIDs)
	current.Emails = normalizeContactEmails(in.Emails)
	current.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.Contact{}, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE contacts
		 SET name=?, notes=?, preferred_account_id=?, preferred_sender_id=?, updated_at=?
		 WHERE id=? AND user_id=?`,
		current.Name, current.Notes, current.PreferredAccountID, current.PreferredSenderID, current.UpdatedAt, current.ID, current.UserID,
	)
	if err != nil {
		return models.Contact{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Contact{}, ErrNotFound
	}
	if err := saveContactDetailsTx(ctx, tx, current); err != nil {
		return models.Contact{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.Contact{}, err
	}
	return s.GetContactByID(ctx, current.UserID, current.ID)
}

func (s *Store) DeleteContact(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM contacts WHERE id=? AND user_id=?`, strings.TrimSpace(id), userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListContactGroups(ctx context.Context, userID, query string) ([]models.ContactGroup, error) {
	ids, err := s.listContactGroupIDs(ctx, userID, query)
	if err != nil {
		return nil, err
	}
	return s.loadContactGroupsByIDs(ctx, userID, ids)
}

func (s *Store) GetContactGroupByID(ctx context.Context, userID, id string) (models.ContactGroup, error) {
	items, err := s.loadContactGroupsByIDs(ctx, userID, []string{strings.TrimSpace(id)})
	if err != nil {
		return models.ContactGroup{}, err
	}
	if len(items) == 0 {
		return models.ContactGroup{}, ErrNotFound
	}
	return items[0], nil
}

func (s *Store) CreateContactGroup(ctx context.Context, in models.ContactGroup) (models.ContactGroup, error) {
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		in.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	in.CreatedAt = now
	in.UpdatedAt = now
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.MemberContactIDs = normalizeContactStrings(in.MemberContactIDs)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ContactGroup{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO contact_groups(id,user_id,name,description,created_at,updated_at)
		 VALUES(?,?,?,?,?,?)`,
		in.ID, in.UserID, in.Name, in.Description, in.CreatedAt, in.UpdatedAt,
	); err != nil {
		if isUniqueConstraintErr(err) {
			return models.ContactGroup{}, ErrConflict
		}
		return models.ContactGroup{}, err
	}
	if err := saveContactGroupMembersTx(ctx, tx, in.UserID, in.ID, in.MemberContactIDs); err != nil {
		return models.ContactGroup{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ContactGroup{}, err
	}
	return s.GetContactGroupByID(ctx, in.UserID, in.ID)
}

func (s *Store) UpdateContactGroup(ctx context.Context, in models.ContactGroup) (models.ContactGroup, error) {
	current, err := s.GetContactGroupByID(ctx, in.UserID, in.ID)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return models.ContactGroup{}, ErrConflict
		}
		return models.ContactGroup{}, err
	}
	current.Name = strings.TrimSpace(in.Name)
	current.Description = strings.TrimSpace(in.Description)
	current.MemberContactIDs = normalizeContactStrings(in.MemberContactIDs)
	current.UpdatedAt = time.Now().UTC()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return models.ContactGroup{}, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE contact_groups SET name=?, description=?, updated_at=? WHERE id=? AND user_id=?`,
		current.Name, current.Description, current.UpdatedAt, current.ID, current.UserID,
	)
	if err != nil {
		return models.ContactGroup{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ContactGroup{}, ErrNotFound
	}
	if err := saveContactGroupMembersTx(ctx, tx, current.UserID, current.ID, current.MemberContactIDs); err != nil {
		return models.ContactGroup{}, err
	}
	if err := tx.Commit(); err != nil {
		return models.ContactGroup{}, err
	}
	return s.GetContactGroupByID(ctx, current.UserID, current.ID)
}

func (s *Store) DeleteContactGroup(ctx context.Context, userID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM contact_groups WHERE id=? AND user_id=?`, strings.TrimSpace(id), userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ContactGroupPrimaryEmails(ctx context.Context, userID, groupID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ce.email
		 FROM contact_group_members cgm
		 JOIN contact_groups cg ON cg.id = cgm.group_id
		 JOIN contacts c ON c.id = cgm.contact_id
		 JOIN contact_emails ce ON ce.contact_id = c.id
		 WHERE cg.user_id=? AND cg.id=? AND ce.is_primary=1
		 ORDER BY c.name COLLATE NOCASE ASC, ce.email ASC`,
		userID,
		strings.TrimSpace(groupID),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0, 16)
	seen := map[string]struct{}{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		key := strings.ToLower(strings.TrimSpace(email))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out, rows.Err()
}

func saveContactDetailsTx(ctx context.Context, tx *sql.Tx, in models.Contact) error {
	if err := ensureContactGroupsOwnedTx(ctx, tx, in.UserID, in.GroupIDs); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM contact_emails WHERE contact_id=?`, in.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM contact_nicknames WHERE contact_id=?`, in.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM contact_group_members WHERE contact_id=?`, in.ID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, item := range in.Emails {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = uuid.NewString()
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO contact_emails(id,contact_id,email,label,is_primary,created_at,updated_at)
			 VALUES(?,?,?,?,?,?,?)`,
			id, in.ID, item.Email, item.Label, boolToInt(item.IsPrimary), now, now,
		); err != nil {
			return err
		}
	}
	for _, nickname := range in.Nicknames {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO contact_nicknames(contact_id,nickname,created_at) VALUES(?,?,?)`,
			in.ID, nickname, now,
		); err != nil {
			return err
		}
	}
	for _, groupID := range in.GroupIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO contact_group_members(group_id,contact_id,created_at) VALUES(?,?,?)`,
			groupID, in.ID, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func saveContactGroupMembersTx(ctx context.Context, tx *sql.Tx, userID, groupID string, memberIDs []string) error {
	if err := ensureContactsOwnedTx(ctx, tx, userID, memberIDs); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM contact_group_members WHERE group_id=?`, groupID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, memberID := range memberIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO contact_group_members(group_id,contact_id,created_at) VALUES(?,?,?)`,
			groupID, memberID, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func ensureContactGroupsOwnedTx(ctx context.Context, tx *sql.Tx, userID string, groupIDs []string) error {
	if len(groupIDs) == 0 {
		return nil
	}
	args := make([]any, 0, len(groupIDs)+1)
	args = append(args, userID)
	for _, id := range groupIDs {
		args = append(args, id)
	}
	var count int
	if err := tx.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(1) FROM contact_groups WHERE user_id=? AND id IN (%s)`, placeholders(len(groupIDs))),
		args...,
	).Scan(&count); err != nil {
		return err
	}
	if count != len(groupIDs) {
		return ErrNotFound
	}
	return nil
}

func ensureContactsOwnedTx(ctx context.Context, tx *sql.Tx, userID string, contactIDs []string) error {
	if len(contactIDs) == 0 {
		return nil
	}
	args := make([]any, 0, len(contactIDs)+1)
	args = append(args, userID)
	for _, id := range contactIDs {
		args = append(args, id)
	}
	var count int
	if err := tx.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(1) FROM contacts WHERE user_id=? AND id IN (%s)`, placeholders(len(contactIDs))),
		args...,
	).Scan(&count); err != nil {
		return err
	}
	if count != len(contactIDs) {
		return ErrNotFound
	}
	return nil
}

func (s *Store) listContactIDs(ctx context.Context, userID, query string) ([]string, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	args := []any{userID}
	sqlQuery := `SELECT DISTINCT c.id
		FROM contacts c
		LEFT JOIN contact_emails ce ON ce.contact_id = c.id
		LEFT JOIN contact_nicknames cn ON cn.contact_id = c.id
		WHERE c.user_id=?`
	if query != "" {
		pattern := "%" + query + "%"
		sqlQuery += ` AND (
			LOWER(c.name) LIKE ?
			OR LOWER(c.notes) LIKE ?
			OR LOWER(ce.email) LIKE ?
			OR LOWER(cn.nickname) LIKE ?
		)`
		args = append(args, pattern, pattern, pattern, pattern)
	}
	sqlQuery += ` ORDER BY CASE WHEN trim(c.name)<>'' THEN c.name ELSE (
			SELECT ce2.email FROM contact_emails ce2 WHERE ce2.contact_id = c.id ORDER BY ce2.is_primary DESC, ce2.email ASC LIMIT 1
		) END COLLATE NOCASE ASC, c.updated_at DESC`
	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, 32)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) loadContactsByIDs(ctx context.Context, userID string, ids []string) ([]models.Contact, error) {
	ids = dedupeOrderedStrings(ids)
	if len(ids) == 0 {
		return []models.Contact{}, nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT id,user_id,name,notes,preferred_account_id,preferred_sender_id,created_at,updated_at
			FROM contacts
			WHERE user_id=? AND id IN (%s)`, placeholders(len(ids))),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := map[string]*models.Contact{}
	for rows.Next() {
		var item models.Contact
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.Name,
			&item.Notes,
			&item.PreferredAccountID,
			&item.PreferredSenderID,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Nicknames = []string{}
		item.Emails = []models.ContactEmail{}
		item.GroupIDs = []string{}
		items[item.ID] = &item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []models.Contact{}, nil
	}
	if err := loadContactEmails(ctx, s.db, items); err != nil {
		return nil, err
	}
	if err := loadContactNicknames(ctx, s.db, items); err != nil {
		return nil, err
	}
	if err := loadContactGroupIDs(ctx, s.db, userID, items); err != nil {
		return nil, err
	}
	out := make([]models.Contact, 0, len(ids))
	for _, id := range ids {
		if item := items[id]; item != nil {
			out = append(out, *item)
		}
	}
	return out, nil
}

func loadContactEmails(ctx context.Context, db *sql.DB, items map[string]*models.Contact) error {
	ids := make([]string, 0, len(items))
	args := make([]any, 0, len(items))
	for id := range items {
		ids = append(ids, id)
		args = append(args, id)
	}
	rows, err := db.QueryContext(ctx,
		fmt.Sprintf(`SELECT id,contact_id,email,label,is_primary
			FROM contact_emails
			WHERE contact_id IN (%s)
			ORDER BY is_primary DESC, email ASC`, placeholders(len(ids))),
		args...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item models.ContactEmail
		var isPrimary int
		if err := rows.Scan(&item.ID, &item.ContactID, &item.Email, &item.Label, &isPrimary); err != nil {
			return err
		}
		item.IsPrimary = isPrimary == 1
		contact := items[item.ContactID]
		if contact == nil {
			continue
		}
		contact.Emails = append(contact.Emails, item)
	}
	return rows.Err()
}

func loadContactNicknames(ctx context.Context, db *sql.DB, items map[string]*models.Contact) error {
	ids := make([]string, 0, len(items))
	args := make([]any, 0, len(items))
	for id := range items {
		ids = append(ids, id)
		args = append(args, id)
	}
	rows, err := db.QueryContext(ctx,
		fmt.Sprintf(`SELECT contact_id,nickname
			FROM contact_nicknames
			WHERE contact_id IN (%s)
			ORDER BY nickname COLLATE NOCASE ASC`, placeholders(len(ids))),
		args...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var contactID string
		var nickname string
		if err := rows.Scan(&contactID, &nickname); err != nil {
			return err
		}
		contact := items[contactID]
		if contact == nil {
			continue
		}
		contact.Nicknames = append(contact.Nicknames, nickname)
	}
	return rows.Err()
}

func loadContactGroupIDs(ctx context.Context, db *sql.DB, userID string, items map[string]*models.Contact) error {
	ids := make([]string, 0, len(items))
	args := make([]any, 0, len(items)+1)
	args = append(args, userID)
	for id := range items {
		ids = append(ids, id)
		args = append(args, id)
	}
	rows, err := db.QueryContext(ctx,
		fmt.Sprintf(`SELECT cgm.contact_id,cgm.group_id
			FROM contact_group_members cgm
			JOIN contact_groups cg ON cg.id = cgm.group_id
			WHERE cg.user_id=? AND cgm.contact_id IN (%s)
			ORDER BY cg.name COLLATE NOCASE ASC`, placeholders(len(ids))),
		args...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var contactID string
		var groupID string
		if err := rows.Scan(&contactID, &groupID); err != nil {
			return err
		}
		contact := items[contactID]
		if contact == nil {
			continue
		}
		contact.GroupIDs = append(contact.GroupIDs, groupID)
	}
	return rows.Err()
}

func (s *Store) listContactGroupIDs(ctx context.Context, userID, query string) ([]string, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	args := []any{userID}
	sqlQuery := `SELECT id FROM contact_groups WHERE user_id=?`
	if query != "" {
		pattern := "%" + query + "%"
		sqlQuery += ` AND (LOWER(name) LIKE ? OR LOWER(description) LIKE ?)`
		args = append(args, pattern, pattern)
	}
	sqlQuery += ` ORDER BY name COLLATE NOCASE ASC, updated_at DESC`
	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0, 16)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) loadContactGroupsByIDs(ctx context.Context, userID string, ids []string) ([]models.ContactGroup, error) {
	ids = dedupeOrderedStrings(ids)
	if len(ids) == 0 {
		return []models.ContactGroup{}, nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT id,user_id,name,description,created_at,updated_at
			FROM contact_groups
			WHERE user_id=? AND id IN (%s)`, placeholders(len(ids))),
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := map[string]*models.ContactGroup{}
	for rows.Next() {
		var item models.ContactGroup
		if err := rows.Scan(&item.ID, &item.UserID, &item.Name, &item.Description, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.MemberContactIDs = []string{}
		items[item.ID] = &item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []models.ContactGroup{}, nil
	}
	memberArgs := make([]any, 0, len(items)+1)
	memberArgs = append(memberArgs, userID)
	for id := range items {
		memberArgs = append(memberArgs, id)
	}
	memberRows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT cgm.group_id,cgm.contact_id
			FROM contact_group_members cgm
			JOIN contact_groups cg ON cg.id = cgm.group_id
			JOIN contacts c ON c.id = cgm.contact_id
			WHERE cg.user_id=? AND cgm.group_id IN (%s)
			ORDER BY c.name COLLATE NOCASE ASC, c.id ASC`, placeholders(len(items))),
		memberArgs...,
	)
	if err != nil {
		return nil, err
	}
	defer memberRows.Close()
	for memberRows.Next() {
		var groupID string
		var contactID string
		if err := memberRows.Scan(&groupID, &contactID); err != nil {
			return nil, err
		}
		group := items[groupID]
		if group == nil {
			continue
		}
		group.MemberContactIDs = append(group.MemberContactIDs, contactID)
		group.MemberCount++
	}
	if err := memberRows.Err(); err != nil {
		return nil, err
	}
	out := make([]models.ContactGroup, 0, len(ids))
	for _, id := range ids {
		if item := items[id]; item != nil {
			out = append(out, *item)
		}
	}
	return out, nil
}
