package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
)

func ApplyMigrationFile(db *sql.DB, path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	if _, err := db.Exec(string(b)); err != nil && !isDuplicateColumnErr(err) {
		return fmt.Errorf("apply migration: %w", err)
	}

	// Backward-compatible patching for early development schema revisions.
	for _, stmt := range []string{
		`ALTER TABLE sessions ADD COLUMN mail_secret TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN mfa_verified_at DATETIME`,
		`ALTER TABLE sessions ADD COLUMN auth_method TEXT NOT NULL DEFAULT 'password'`,
		`ALTER TABLE sessions ADD COLUMN active_account_id TEXT`,
		`ALTER TABLE drafts ADD COLUMN sender_profile_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE users ADD COLUMN provision_state TEXT NOT NULL DEFAULT 'pending'`,
		`ALTER TABLE users ADD COLUMN provision_error TEXT`,
		`ALTER TABLE users ADD COLUMN mail_login TEXT`,
		`ALTER TABLE users ADD COLUMN mfa_preference TEXT NOT NULL DEFAULT 'none'`,
		`ALTER TABLE users ADD COLUMN legacy_mfa_prompt_pending INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN mfa_setup_switch_used INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE users ADD COLUMN mfa_backup_completed INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE users ADD COLUMN mail_secret_enc TEXT`,
		`ALTER TABLE users ADD COLUMN mail_secret_updated_at DATETIME`,
		`ALTER TABLE registrations ADD COLUMN mfa_preference TEXT NOT NULL DEFAULT 'none'`,
		`CREATE TABLE IF NOT EXISTS mfa_trusted_devices (
		   id TEXT PRIMARY KEY,
		   user_id TEXT NOT NULL,
		   token_hash TEXT NOT NULL UNIQUE,
		   ua_hash TEXT NOT NULL DEFAULT '',
		   ip_hint TEXT NOT NULL DEFAULT '',
		   device_label TEXT NOT NULL DEFAULT '',
		   created_at DATETIME NOT NULL,
		   last_used_at DATETIME,
		   expires_at DATETIME NOT NULL,
		   revoked_at DATETIME,
		   FOREIGN KEY(user_id) REFERENCES users(id)
		 )`,
		`CREATE INDEX IF NOT EXISTS idx_mfa_trusted_user ON mfa_trusted_devices(user_id, expires_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mfa_trusted_active ON mfa_trusted_devices(user_id, revoked_at, expires_at)`,
		`ALTER TABLE user_preferences ADD COLUMN default_sender_id TEXT NOT NULL DEFAULT ''`,
		`UPDATE users
		 SET legacy_mfa_prompt_pending = 1
		 WHERE status = 'active'
		   AND NOT EXISTS (
		     SELECT 1
		     FROM mfa_totp t
		     WHERE t.user_id = users.id
		       AND t.enabled = 1
		       AND length(trim(coalesce(t.secret_enc, ''))) > 0
		   )
		   AND NOT EXISTS (
		     SELECT 1
		     FROM mfa_webauthn_credentials w
		     WHERE w.user_id = users.id
		   )`,
	} {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumnErr(err) && !isNoSuchTableErr(err) {
			return fmt.Errorf("apply compatibility migration %q: %w", stmt, err)
		}
	}
	return nil
}

func isDuplicateColumnErr(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}

func isNoSuchTableErr(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table")
}
