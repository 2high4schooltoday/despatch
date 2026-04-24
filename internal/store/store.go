package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"despatch/internal/models"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("conflict")
var ErrUserStateConflict = errors.New("registration user state conflict")

type Store struct {
	db *sql.DB
}

type SettingValue struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) CreateUser(ctx context.Context, email, passwordHash, role string, status models.UserStatus) (models.User, error) {
	return s.CreateUserWithMFA(ctx, email, passwordHash, role, status, "none")
}

func normalizeMFAPreference(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "totp":
		return "totp"
	case "webauthn":
		return "webauthn"
	default:
		return "none"
	}
}

func scanUserCore(scanner interface{ Scan(dest ...any) error }) (models.User, error) {
	var u models.User
	var approvedAt, lastLogin sql.NullTime
	var approvedBy, provisionErr, recoveryEmail, mailLogin, mfaPref sql.NullString
	var legacyPrompt, switchUsed, backupCompleted int
	err := scanner.Scan(
		&u.ID,
		&u.Email,
		&recoveryEmail,
		&mailLogin,
		&mfaPref,
		&legacyPrompt,
		&switchUsed,
		&backupCompleted,
		&u.PasswordHash,
		&u.Role,
		&u.Status,
		&u.ProvisionState,
		&provisionErr,
		&u.CreatedAt,
		&approvedAt,
		&approvedBy,
		&lastLogin,
	)
	if err != nil {
		return models.User{}, err
	}
	u.MFAPreference = normalizeMFAPreference(mfaPref.String)
	u.LegacyMFAPromptPending = legacyPrompt == 1
	u.MFASetupSwitchUsed = switchUsed == 1
	u.MFABackupCompleted = backupCompleted == 1
	if approvedAt.Valid {
		t := approvedAt.Time
		u.ApprovedAt = &t
	}
	if approvedBy.Valid {
		v := strings.TrimSpace(approvedBy.String)
		if v != "" {
			u.ApprovedBy = &v
		}
	}
	if lastLogin.Valid {
		t := lastLogin.Time
		u.LastLoginAt = &t
	}
	if provisionErr.Valid {
		v := provisionErr.String
		u.ProvisionError = &v
	}
	if recoveryEmail.Valid {
		v := strings.ToLower(strings.TrimSpace(recoveryEmail.String))
		if v != "" {
			u.RecoveryEmail = &v
		}
	}
	if mailLogin.Valid {
		v := strings.TrimSpace(mailLogin.String)
		if v != "" {
			u.MailLogin = &v
		}
	}
	return u, nil
}

func (s *Store) CreateUserWithMFA(ctx context.Context, email, passwordHash, role string, status models.UserStatus, mfaPreference string) (models.User, error) {
	return s.CreateUserWithMFARecovery(ctx, email, passwordHash, role, status, mfaPreference, email)
}

func (s *Store) CreateUserWithMFARecovery(ctx context.Context, email, passwordHash, role string, status models.UserStatus, mfaPreference, recoveryEmail string) (models.User, error) {
	now := time.Now().UTC()
	recovery := strings.ToLower(strings.TrimSpace(recoveryEmail))
	preference := normalizeMFAPreference(mfaPreference)
	u := models.User{
		ID:                     uuid.NewString(),
		Email:                  email,
		PasswordHash:           passwordHash,
		Role:                   role,
		Status:                 status,
		ProvisionState:         "pending",
		CreatedAt:              now,
		MFAPreference:          preference,
		LegacyMFAPromptPending: false,
		MFASetupSwitchUsed:     false,
		MFABackupCompleted:     preference == "none",
	}
	if recovery != "" {
		u.RecoveryEmail = &recovery
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users(id,email,recovery_email,mail_login,mfa_preference,legacy_mfa_prompt_pending,mfa_setup_switch_used,mfa_backup_completed,password_hash,role,status,provision_state,provision_error,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		u.ID,
		u.Email,
		u.RecoveryEmail,
		nil,
		u.MFAPreference,
		0,
		0,
		boolToInt(u.MFABackupCompleted),
		u.PasswordHash,
		u.Role,
		u.Status,
		u.ProvisionState,
		nil,
		u.CreatedAt,
	)
	return u, err
}

func (s *Store) EnsureAdmin(ctx context.Context, email, passwordHash string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || passwordHash == "" {
		return nil
	}
	u, err := s.GetUserByEmail(ctx, email)
	if err == ErrNotFound {
		now := time.Now().UTC()
		_, err = s.db.ExecContext(ctx,
			`INSERT INTO users(id,email,recovery_email,mail_login,password_hash,role,status,provision_state,provision_error,created_at,approved_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
			uuid.NewString(), email, email, nil, passwordHash, "admin", models.UserActive, "ok", nil, now, now,
		)
		return err
	}
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET role='admin', status='active', password_hash=?, provision_state='ok', provision_error=NULL, recovery_email=COALESCE(NULLIF(trim(recovery_email),''), email) WHERE id=?`,
		passwordHash, u.ID,
	)
	return err
}

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM users WHERE role='admin'`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *Store) UpsertSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, time.Now().UTC(),
	)
	return err
}

func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key=?`, key)
	return err
}

func (s *Store) DeleteSettingsByPrefixOlderThan(ctx context.Context, prefix string, before time.Time) error {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key LIKE ? AND updated_at < ?`, prefix+"%", before.UTC())
	return err
}

func (s *Store) ListSettingsByPrefix(ctx context.Context, prefix string, limit int) ([]SettingValue, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT key,value,updated_at
		 FROM settings
		 WHERE key LIKE ?
		 ORDER BY updated_at DESC
		 LIMIT ?`,
		prefix+"%",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SettingValue, 0, limit)
	for rows.Next() {
		var item SettingValue
		if err := rows.Scan(&item.Key, &item.Value, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (models.User, error) {
	u, err := scanUserCore(s.db.QueryRowContext(ctx,
		`SELECT id,email,recovery_email,mail_login,mfa_preference,legacy_mfa_prompt_pending,mfa_setup_switch_used,mfa_backup_completed,password_hash,role,status,provision_state,provision_error,created_at,approved_at,approved_by,last_login_at
		 FROM users
		 WHERE email=?`,
		email,
	))
	if err == sql.ErrNoRows {
		return models.User{}, ErrNotFound
	}
	if err != nil {
		return models.User{}, err
	}
	return u, nil
}

func (s *Store) GetUserByRecoveryEmail(ctx context.Context, recoveryEmail string) (models.User, error) {
	recoveryEmail = strings.ToLower(strings.TrimSpace(recoveryEmail))
	if recoveryEmail == "" {
		return models.User{}, ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,email,recovery_email,mail_login,mfa_preference,legacy_mfa_prompt_pending,mfa_setup_switch_used,mfa_backup_completed,password_hash,role,status,provision_state,provision_error,created_at,approved_at,approved_by,last_login_at
		 FROM users
		 WHERE lower(trim(coalesce(recovery_email, ''))) = ?
		 LIMIT 2`,
		recoveryEmail,
	)
	if err != nil {
		return models.User{}, err
	}
	defer rows.Close()

	if !rows.Next() {
		return models.User{}, ErrNotFound
	}
	u, err := scanUserCore(rows)
	if err != nil {
		return models.User{}, err
	}
	if rows.Next() {
		return models.User{}, ErrConflict
	}
	if err := rows.Err(); err != nil {
		return models.User{}, err
	}
	return u, nil
}

func (s *Store) GetUserByID(ctx context.Context, id string) (models.User, error) {
	u, err := scanUserCore(s.db.QueryRowContext(ctx,
		`SELECT id,email,recovery_email,mail_login,mfa_preference,legacy_mfa_prompt_pending,mfa_setup_switch_used,mfa_backup_completed,password_hash,role,status,provision_state,provision_error,created_at,approved_at,approved_by,last_login_at
		 FROM users
		 WHERE id=?`,
		id,
	))
	if err == sql.ErrNoRows {
		return models.User{}, ErrNotFound
	}
	if err != nil {
		return models.User{}, err
	}
	return u, nil
}

func (s *Store) UpdateUserStatus(ctx context.Context, userID string, status models.UserStatus, approver *string) error {
	now := time.Now().UTC()
	if status == models.UserActive && approver != nil {
		_, err := s.db.ExecContext(ctx,
			`UPDATE users SET status=?, approved_at=?, approved_by=? WHERE id=?`,
			status, now, *approver, userID,
		)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE users SET status=? WHERE id=?`, status, userID)
	return err
}

func (s *Store) UpdateUserPasswordHash(ctx context.Context, userID, passwordHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE id=?`, passwordHash, userID)
	return err
}

func (s *Store) UpdateUserMailLogin(ctx context.Context, userID, mailLogin string) error {
	mailLogin = strings.TrimSpace(mailLogin)
	if mailLogin == "" {
		_, err := s.db.ExecContext(ctx, `UPDATE users SET mail_login=NULL WHERE id=?`, userID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE users SET mail_login=? WHERE id=?`, mailLogin, userID)
	return err
}

func (s *Store) UpsertUserMailSecret(ctx context.Context, userID, mailSecretEnc string) error {
	mailSecretEnc = strings.TrimSpace(mailSecretEnc)
	now := time.Now().UTC()
	if mailSecretEnc == "" {
		_, err := s.db.ExecContext(ctx, `UPDATE users SET mail_secret_enc=NULL, mail_secret_updated_at=NULL WHERE id=?`, userID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE users SET mail_secret_enc=?, mail_secret_updated_at=? WHERE id=?`, mailSecretEnc, now, userID)
	return err
}

func (s *Store) GetUserMailSecret(ctx context.Context, userID string) (string, bool, error) {
	var raw sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT mail_secret_enc FROM users WHERE id=?`, userID).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return "", false, ErrNotFound
		}
		return "", false, err
	}
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return "", false, nil
	}
	return strings.TrimSpace(raw.String), true, nil
}

func (s *Store) UpdateUserRecoveryEmail(ctx context.Context, userID, recoveryEmail string) error {
	recoveryEmail = strings.ToLower(strings.TrimSpace(recoveryEmail))
	if recoveryEmail == "" {
		_, err := s.db.ExecContext(ctx, `UPDATE users SET recovery_email=NULL WHERE id=?`, userID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE users SET recovery_email=? WHERE id=?`, recoveryEmail, userID)
	return err
}

func (s *Store) GetUserMFAPreference(ctx context.Context, userID string) (string, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT mfa_preference FROM users WHERE id=?`, userID).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return normalizeMFAPreference(raw), nil
}

func (s *Store) UpdateUserMFAPreference(ctx context.Context, userID, preference string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET mfa_preference=? WHERE id=?`, normalizeMFAPreference(preference), userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetLegacyMFAPromptPending(ctx context.Context, userID string, pending bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET legacy_mfa_prompt_pending=? WHERE id=?`, boolToInt(pending), userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) MarkMFASetupSwitchUsed(ctx context.Context, userID string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET mfa_setup_switch_used=1 WHERE id=?`, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetMFASetupSwitchUsed(ctx context.Context, userID string, used bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET mfa_setup_switch_used=? WHERE id=?`, boolToInt(used), userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetUserMFABackupCompleted(ctx context.Context, userID string, completed bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET mfa_backup_completed=? WHERE id=?`, boolToInt(completed), userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) TouchUserLastLogin(ctx context.Context, userID string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET last_login_at=? WHERE id=?`, at, userID)
	return err
}

func (s *Store) UpdateProvisionState(ctx context.Context, userID, state string, errMsg *string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET provision_state=?, provision_error=? WHERE id=?`, state, errMsg, userID)
	return err
}

func (s *Store) ListUsers(ctx context.Context, query models.UserQuery) ([]models.User, int, error) {
	where := []string{"lower(trim(coalesce(status, ''))) <> 'rejected'"}
	args := make([]any, 0, 12)

	q := strings.TrimSpace(query.Q)
	if q != "" {
		needle := "%" + strings.ToLower(q) + "%"
		where = append(where, "(lower(email) LIKE ? OR lower(role) LIKE ? OR lower(status) LIKE ? OR lower(provision_state) LIKE ?)")
		args = append(args, needle, needle, needle, needle)
	}
	if status := strings.TrimSpace(strings.ToLower(query.Status)); status != "" && status != "all" {
		where = append(where, "lower(status)=?")
		args = append(args, status)
	}
	if role := strings.TrimSpace(strings.ToLower(query.Role)); role != "" && role != "all" {
		where = append(where, "lower(role)=?")
		args = append(args, role)
	}
	if ps := strings.TrimSpace(strings.ToLower(query.ProvisionState)); ps != "" && ps != "all" {
		where = append(where, "lower(provision_state)=?")
		args = append(args, ps)
	}

	whereSQL := strings.Join(where, " AND ")

	countArgs := append([]any{}, args...)
	countSQL := fmt.Sprintf("SELECT COUNT(1) FROM users WHERE %s", whereSQL)
	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortExpr := sanitizeUserSort(query.Sort)
	orderExpr := sanitizeSortOrder(query.Order)
	limit := clampLimit(query.Limit)
	offset := clampOffset(query.Offset)

	listSQL := fmt.Sprintf(
		`SELECT id,email,recovery_email,mail_login,mfa_preference,legacy_mfa_prompt_pending,mfa_setup_switch_used,mfa_backup_completed,password_hash,role,status,provision_state,provision_error,created_at,approved_at,approved_by,last_login_at
		 FROM users
		 WHERE %s
		 ORDER BY %s %s
		 LIMIT ? OFFSET ?`,
		whereSQL,
		sortExpr,
		orderExpr,
	)
	listArgs := append(args, limit, offset)
	rows, err := s.db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]models.User, 0, limit)
	for rows.Next() {
		u, err := scanUserCore(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, rows.Err()
}

func (s *Store) CreateRegistration(ctx context.Context, email, ip, uaHash string, captchaOK bool) (models.Registration, error) {
	return s.CreateRegistrationWithMFA(ctx, email, ip, uaHash, captchaOK, "none")
}

func (s *Store) CreateRegistrationWithMFA(ctx context.Context, email, ip, uaHash string, captchaOK bool, mfaPreference string) (models.Registration, error) {
	now := time.Now().UTC()
	r := models.Registration{
		ID:            uuid.NewString(),
		Email:         email,
		MFAPreference: normalizeMFAPreference(mfaPreference),
		SourceIP:      ip,
		UserAgentHash: uaHash,
		CaptchaOK:     captchaOK,
		Status:        "pending",
		CreatedAt:     now,
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO registrations(id,email,mfa_preference,source_ip,user_agent_hash,captcha_ok,status,created_at) VALUES(?,?,?,?,?,?,?,?)`,
		r.ID, r.Email, r.MFAPreference, r.SourceIP, r.UserAgentHash, boolToInt(captchaOK), r.Status, r.CreatedAt,
	)
	return r, err
}

func (s *Store) GetRegistrationByID(ctx context.Context, id string) (models.Registration, error) {
	var r models.Registration
	var cap int
	var decidedAt sql.NullTime
	var decidedBy, reason, mfaPreference sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,email,mfa_preference,source_ip,user_agent_hash,captcha_ok,status,created_at,decided_at,decided_by,reason
		 FROM registrations
		 WHERE id=?`,
		id,
	).Scan(&r.ID, &r.Email, &mfaPreference, &r.SourceIP, &r.UserAgentHash, &cap, &r.Status, &r.CreatedAt, &decidedAt, &decidedBy, &reason)
	if err == sql.ErrNoRows {
		return models.Registration{}, ErrNotFound
	}
	if err != nil {
		return models.Registration{}, err
	}
	r.CaptchaOK = cap == 1
	r.MFAPreference = normalizeMFAPreference(mfaPreference.String)
	if decidedAt.Valid {
		t := decidedAt.Time
		r.DecidedAt = &t
	}
	if decidedBy.Valid {
		sv := decidedBy.String
		r.DecidedBy = &sv
	}
	if reason.Valid {
		sv := reason.String
		r.Reason = &sv
	}
	return r, nil
}

func (s *Store) ListRegistrations(ctx context.Context, query models.RegistrationQuery) ([]models.Registration, int, error) {
	where := make([]string, 0, 4)
	args := make([]any, 0, 8)

	status := strings.TrimSpace(strings.ToLower(query.Status))
	if status == "" {
		status = "pending"
	}
	if status != "all" {
		where = append(where, "lower(status)=?")
		args = append(args, status)
	}

	q := strings.TrimSpace(query.Q)
	if q != "" {
		needle := "%" + strings.ToLower(q) + "%"
		where = append(where, "(lower(email) LIKE ? OR lower(coalesce(reason,'')) LIKE ?)")
		args = append(args, needle, needle)
	}

	whereSQL := "1=1"
	if len(where) > 0 {
		whereSQL = strings.Join(where, " AND ")
	}

	countSQL := fmt.Sprintf("SELECT COUNT(1) FROM registrations WHERE %s", whereSQL)
	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, append([]any{}, args...)...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortExpr := sanitizeRegistrationSort(query.Sort)
	orderExpr := sanitizeSortOrder(query.Order)
	limit := clampLimit(query.Limit)
	offset := clampOffset(query.Offset)

	listSQL := fmt.Sprintf(
		`SELECT id,email,mfa_preference,source_ip,user_agent_hash,captcha_ok,status,created_at,decided_at,decided_by,reason
		 FROM registrations
		 WHERE %s
		 ORDER BY %s %s
		 LIMIT ? OFFSET ?`,
		whereSQL,
		sortExpr,
		orderExpr,
	)
	rows, err := s.db.QueryContext(ctx, listSQL, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]models.Registration, 0, limit)
	for rows.Next() {
		var r models.Registration
		var cap int
		var decidedAt sql.NullTime
		var decidedBy, reason, mfaPreference sql.NullString
		if err := rows.Scan(&r.ID, &r.Email, &mfaPreference, &r.SourceIP, &r.UserAgentHash, &cap, &r.Status, &r.CreatedAt, &decidedAt, &decidedBy, &reason); err != nil {
			return nil, 0, err
		}
		r.CaptchaOK = cap == 1
		r.MFAPreference = normalizeMFAPreference(mfaPreference.String)
		if decidedAt.Valid {
			t := decidedAt.Time
			r.DecidedAt = &t
		}
		if decidedBy.Valid {
			sv := decidedBy.String
			r.DecidedBy = &sv
		}
		if reason.Valid {
			sv := reason.String
			r.Reason = &sv
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *Store) SetRegistrationDecision(ctx context.Context, regID, status, decidedBy, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var email, currentStatus string
	if err := tx.QueryRowContext(ctx, `SELECT email,status FROM registrations WHERE id=?`, regID).Scan(&email, &currentStatus); err != nil {
		if err == sql.ErrNoRows {
			return ErrConflict
		}
		return err
	}
	if currentStatus != "pending" {
		return ErrConflict
	}
	// Keep one decided row per normalized email+status to avoid UNIQUE(email,status)
	// collisions when a new pending registration is decided again for the same account.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM registrations
		  WHERE id<>?
		    AND status=?
		    AND lower(trim(email))=lower(trim(?))`,
		regID, status, email,
	); err != nil {
		return err
	}

	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx,
		`UPDATE registrations SET status=?,decided_at=?,decided_by=?,reason=? WHERE id=? AND status='pending'`,
		status, now, decidedBy, reason, regID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrConflict
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// RejectRegistrationAndDeletePendingUser marks a pending registration as rejected and
// removes the associated pending user in one transaction.
func (s *Store) RejectRegistrationAndDeletePendingUser(ctx context.Context, regID, decidedBy, reason string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var email, regStatus string
	if err := tx.QueryRowContext(ctx, `SELECT email,status FROM registrations WHERE id=?`, regID).Scan(&email, &regStatus); err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	if regStatus != "pending" {
		return "", ErrConflict
	}

	var userID string
	var userStatus models.UserStatus
	userLookupErr := tx.QueryRowContext(ctx, `SELECT id,status FROM users WHERE lower(email)=lower(?) ORDER BY id LIMIT 1`, email).Scan(&userID, &userStatus)
	if userLookupErr != nil && userLookupErr != sql.ErrNoRows {
		return "", userLookupErr
	}
	if userLookupErr == nil && userStatus != models.UserPending {
		return "", ErrUserStateConflict
	}

	now := time.Now().UTC()
	// If a previous rejection exists for the same normalized email, remove it
	// before deciding this pending row to avoid UNIQUE(email,status) conflicts.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM registrations
		  WHERE id<>?
		    AND status='rejected'
		    AND lower(trim(email))=lower(trim(?))`,
		regID, email,
	); err != nil {
		return "", err
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE registrations SET status=?,decided_at=?,decided_by=?,reason=? WHERE id=? AND status='pending'`,
		"rejected", now, decidedBy, reason, regID,
	)
	if err != nil {
		return "", err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return "", err
	}
	if rows == 0 {
		return "", ErrConflict
	}

	if strings.TrimSpace(userID) != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id=?`, userID); err != nil {
			return "", err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE user_id=?`, userID); err != nil {
			return "", err
		}
		res, err = tx.ExecContext(ctx, `DELETE FROM users WHERE id=?`, userID)
		if err != nil {
			return "", err
		}
		rows, err = res.RowsAffected()
		if err != nil {
			return "", err
		}
		if rows == 0 {
			return "", ErrNotFound
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return userID, nil
}

func (s *Store) CreateSession(ctx context.Context, sess models.Session) error {
	authMethod := strings.TrimSpace(sess.AuthMethod)
	if authMethod == "" {
		authMethod = "password"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions(id,user_id,token_hash,mail_secret,ip_hint,user_agent_hash,auth_method,mfa_verified_at,active_account_id,expires_at,idle_expires_at,created_at,last_seen_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sess.ID,
		sess.UserID,
		sess.TokenHash,
		sess.MailSecret,
		sess.IPHint,
		sess.UserAgentHash,
		authMethod,
		sess.MFAVerifiedAt,
		sess.ActiveAccountID,
		sess.ExpiresAt,
		sess.IdleExpiresAt,
		sess.CreatedAt,
		sess.LastSeenAt,
	)
	return err
}

func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (models.Session, error) {
	var sess models.Session
	var revoked sql.NullTime
	var mfaVerifiedAt sql.NullTime
	var activeAccountID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,token_hash,mail_secret,ip_hint,user_agent_hash,auth_method,mfa_verified_at,active_account_id,expires_at,idle_expires_at,created_at,last_seen_at,revoked_at FROM sessions WHERE token_hash=?`,
		tokenHash,
	).Scan(
		&sess.ID,
		&sess.UserID,
		&sess.TokenHash,
		&sess.MailSecret,
		&sess.IPHint,
		&sess.UserAgentHash,
		&sess.AuthMethod,
		&mfaVerifiedAt,
		&activeAccountID,
		&sess.ExpiresAt,
		&sess.IdleExpiresAt,
		&sess.CreatedAt,
		&sess.LastSeenAt,
		&revoked,
	)
	if err == sql.ErrNoRows {
		return models.Session{}, ErrNotFound
	}
	if err != nil {
		return models.Session{}, err
	}
	if revoked.Valid {
		t := revoked.Time
		sess.RevokedAt = &t
	}
	if mfaVerifiedAt.Valid {
		t := mfaVerifiedAt.Time
		sess.MFAVerifiedAt = &t
	}
	if activeAccountID.Valid {
		v := strings.TrimSpace(activeAccountID.String)
		if v != "" {
			sess.ActiveAccountID = &v
		}
	}
	if strings.TrimSpace(sess.AuthMethod) == "" {
		sess.AuthMethod = "password"
	}
	return sess, nil
}

func (s *Store) TouchSession(ctx context.Context, id string, idleExpiry time.Time) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at=?, idle_expires_at=? WHERE id=?`, now, idleExpiry, id)
	return err
}

func (s *Store) UpdateSessionMailSecret(ctx context.Context, sessionID, mailSecret string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE sessions SET mail_secret=? WHERE id=?`, strings.TrimSpace(mailSecret), sessionID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RevokeSession(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE id=?`, now, id)
	return err
}

func (s *Store) RevokeUserSessions(ctx context.Context, userID string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`, now, userID)
	return err
}

func (s *Store) SetSessionMFAVerified(ctx context.Context, sessionID, authMethod string) error {
	authMethod = strings.TrimSpace(authMethod)
	if authMethod == "" {
		authMethod = "mfa"
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET mfa_verified_at=?, auth_method=? WHERE id=?`,
		time.Now().UTC(),
		authMethod,
		sessionID,
	)
	return err
}

func (s *Store) ClearSessionMFAVerified(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET mfa_verified_at=NULL, auth_method='password' WHERE id=?`,
		sessionID,
	)
	return err
}

func (s *Store) InsertAudit(ctx context.Context, actorID, action, target, metadata string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO admin_audit_log(id,actor_user_id,action,target,metadata_json,created_at) VALUES(?,?,?,?,?,?)`,
		uuid.NewString(), actorID, action, target, metadata, time.Now().UTC(),
	)
	return err
}

func (s *Store) ListAudit(ctx context.Context, query models.AuditQuery) ([]models.AuditEntry, int, error) {
	where := make([]string, 0, 8)
	args := make([]any, 0, 16)

	if q := strings.TrimSpace(query.Q); q != "" {
		needle := "%" + strings.ToLower(q) + "%"
		where = append(where, "(lower(a.action) LIKE ? OR lower(a.target) LIKE ? OR lower(a.metadata_json) LIKE ? OR lower(coalesce(u.email,'')) LIKE ?)")
		args = append(args, needle, needle, needle, needle)
	}
	if action := strings.TrimSpace(strings.ToLower(query.Action)); action != "" && action != "all" {
		where = append(where, "lower(a.action)=?")
		args = append(args, action)
	}
	if actor := strings.TrimSpace(strings.ToLower(query.Actor)); actor != "" {
		where = append(where, "lower(coalesce(u.email,'')) LIKE ?")
		args = append(args, "%"+actor+"%")
	}
	if target := strings.TrimSpace(strings.ToLower(query.Target)); target != "" {
		where = append(where, "lower(a.target) LIKE ?")
		args = append(args, "%"+target+"%")
	}
	if !query.From.IsZero() {
		where = append(where, "a.created_at >= ?")
		args = append(args, query.From.UTC())
	}
	if !query.To.IsZero() {
		where = append(where, "a.created_at <= ?")
		args = append(args, query.To.UTC())
	}

	whereSQL := "1=1"
	if len(where) > 0 {
		whereSQL = strings.Join(where, " AND ")
	}

	countSQL := fmt.Sprintf(
		`SELECT COUNT(1)
		 FROM admin_audit_log a
		 LEFT JOIN users u ON u.id = a.actor_user_id
		 WHERE %s`,
		whereSQL,
	)
	var total int
	if err := s.db.QueryRowContext(ctx, countSQL, append([]any{}, args...)...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sortExpr := sanitizeAuditSort(query.Sort)
	orderExpr := sanitizeSortOrder(query.Order)
	limit := clampLimit(query.Limit)
	offset := clampOffset(query.Offset)
	listSQL := fmt.Sprintf(
		`SELECT a.id,a.actor_user_id,coalesce(u.email,''),a.action,a.target,a.metadata_json,a.created_at
		 FROM admin_audit_log a
		 LEFT JOIN users u ON u.id = a.actor_user_id
		 WHERE %s
		 ORDER BY %s %s
		 LIMIT ? OFFSET ?`,
		whereSQL,
		sortExpr,
		orderExpr,
	)
	rows, err := s.db.QueryContext(ctx, listSQL, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]models.AuditEntry, 0, limit)
	for rows.Next() {
		var e models.AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorUserID, &e.ActorEmail, &e.Action, &e.Target, &e.MetadataJSON, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return 25
	case limit > 100:
		return 100
	default:
		return limit
	}
}

func clampOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func sanitizeSortOrder(order string) string {
	if strings.EqualFold(strings.TrimSpace(order), "asc") {
		return "ASC"
	}
	return "DESC"
}

func sanitizeRegistrationSort(sort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "email":
		return "email"
	case "status":
		return "status"
	case "decided_at":
		return "decided_at"
	case "created_at":
		fallthrough
	default:
		return "created_at"
	}
}

func sanitizeUserSort(sort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "email":
		return "email"
	case "role":
		return "role"
	case "status":
		return "status"
	case "provision_state":
		return "provision_state"
	case "last_login_at":
		return "last_login_at"
	case "created_at":
		fallthrough
	default:
		return "created_at"
	}
}

func sanitizeAuditSort(sort string) string {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "action":
		return "a.action"
	case "actor":
		return "u.email"
	case "target":
		return "a.target"
	case "created_at":
		fallthrough
	default:
		return "a.created_at"
	}
}

func (s *Store) CreatePasswordResetToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (models.PasswordResetToken, error) {
	t := models.PasswordResetToken{ID: uuid.NewString(), UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt, CreatedAt: time.Now().UTC()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO password_reset_tokens(id,user_id,token_hash,expires_at,created_at) VALUES(?,?,?,?,?)`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt, t.CreatedAt,
	)
	return t, err
}

func (s *Store) InvalidatePasswordResetToken(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE password_reset_tokens SET used_at=?, reserved_at=NULL, reservation_id=NULL WHERE token_hash=? AND used_at IS NULL`,
		time.Now().UTC(), tokenHash,
	)
	return err
}

func (s *Store) ReservePasswordResetToken(ctx context.Context, tokenHash, reservationID string, lease time.Duration) (models.PasswordResetToken, error) {
	if strings.TrimSpace(reservationID) == "" {
		return models.PasswordResetToken{}, fmt.Errorf("reservation_id is required")
	}
	now := time.Now().UTC()
	if lease <= 0 {
		lease = 30 * time.Second
	}
	staleBefore := now.Add(-lease)
	res, err := s.db.ExecContext(ctx,
		`UPDATE password_reset_tokens
		 SET reserved_at=?, reservation_id=?
		 WHERE token_hash=?
		   AND used_at IS NULL
		   AND expires_at>?
		   AND (reserved_at IS NULL OR reserved_at<=?)`,
		now, strings.TrimSpace(reservationID), tokenHash, now, staleBefore,
	)
	if err != nil {
		return models.PasswordResetToken{}, err
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		return models.PasswordResetToken{}, ErrNotFound
	}
	return s.getPasswordResetTokenByHash(ctx, tokenHash)
}

func (s *Store) ReleasePasswordResetTokenReservation(ctx context.Context, tokenHash, reservationID string) error {
	if strings.TrimSpace(reservationID) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE password_reset_tokens
		 SET reserved_at=NULL, reservation_id=NULL
		 WHERE token_hash=? AND used_at IS NULL AND reservation_id=?`,
		tokenHash, strings.TrimSpace(reservationID),
	)
	return err
}

func (s *Store) ConsumePasswordResetToken(ctx context.Context, tokenHash string) (models.PasswordResetToken, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE password_reset_tokens
		 SET used_at=?, reserved_at=NULL, reservation_id=NULL
		 WHERE token_hash=? AND used_at IS NULL AND expires_at>?`,
		now, tokenHash, now,
	)
	if err != nil {
		return models.PasswordResetToken{}, err
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		return models.PasswordResetToken{}, ErrNotFound
	}
	return s.getPasswordResetTokenByHash(ctx, tokenHash)
}

func (s *Store) ConsumeReservedPasswordResetToken(ctx context.Context, tokenHash, reservationID string) (models.PasswordResetToken, error) {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE password_reset_tokens
		 SET used_at=?, reserved_at=NULL, reservation_id=NULL
		 WHERE token_hash=? AND used_at IS NULL AND expires_at>? AND reservation_id=?`,
		now, tokenHash, now, strings.TrimSpace(reservationID),
	)
	if err != nil {
		return models.PasswordResetToken{}, err
	}
	affected, _ := res.RowsAffected()
	if affected != 1 {
		return models.PasswordResetToken{}, ErrNotFound
	}
	return s.getPasswordResetTokenByHash(ctx, tokenHash)
}

func (s *Store) getPasswordResetTokenByHash(ctx context.Context, tokenHash string) (models.PasswordResetToken, error) {
	var t models.PasswordResetToken
	var used, reserved sql.NullTime
	var reservationID sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,token_hash,reservation_id,reserved_at,expires_at,used_at,created_at FROM password_reset_tokens WHERE token_hash=?`,
		tokenHash,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &reservationID, &reserved, &t.ExpiresAt, &used, &t.CreatedAt)
	if err == sql.ErrNoRows {
		return models.PasswordResetToken{}, ErrNotFound
	}
	if err != nil {
		return models.PasswordResetToken{}, err
	}
	if reservationID.Valid {
		v := strings.TrimSpace(reservationID.String)
		if v != "" {
			t.ReservationID = &v
		}
	}
	if reserved.Valid {
		tm := reserved.Time
		t.ReservedAt = &tm
	}
	if used.Valid {
		tm := used.Time
		t.UsedAt = &tm
	}
	return t, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) IncrementRateEvent(ctx context.Context, key, route string, windowStart time.Time) (int, error) {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO rate_limit_events(id,key,route,window_start,count,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?)
		 ON CONFLICT(key, route, window_start)
		 DO UPDATE SET count = rate_limit_events.count + 1, updated_at = excluded.updated_at`,
		uuid.NewString(), key, route, windowStart, 1, now, now,
	)
	if err != nil {
		return 0, err
	}

	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count FROM rate_limit_events WHERE key=? AND route=? AND window_start=?`, key, route, windowStart).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) DeleteRateEvents(ctx context.Context, key, route string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rate_limit_events WHERE key=? AND route=?`, key, route)
	return err
}

func (s *Store) CleanupRateEventsBefore(ctx context.Context, before time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rate_limit_events WHERE window_start < ?`, before)
	return err
}
