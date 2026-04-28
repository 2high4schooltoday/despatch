package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"despatch/internal/store"
)

func TestApplyMigrationFileAddsCompatibilityColumnsForLegacySchema(t *testing.T) {
	sqdb, err := OpenSQLite(filepath.Join(t.TempDir(), "legacy.db"), 1, 1, time.Minute)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqdb.Close() })

	legacySchema := `
CREATE TABLE users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'user',
  status TEXT NOT NULL CHECK (status IN ('pending','active','suspended','rejected')),
  created_at DATETIME NOT NULL,
  approved_at DATETIME,
  approved_by TEXT,
  last_login_at DATETIME
);
CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  ip_hint TEXT,
  user_agent_hash TEXT,
  expires_at DATETIME NOT NULL,
  idle_expires_at DATETIME NOT NULL,
  created_at DATETIME NOT NULL,
  last_seen_at DATETIME NOT NULL,
  revoked_at DATETIME
);
`
	if _, err := sqdb.Exec(legacySchema); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	for _, migration := range []string{
		filepath.Join("..", "..", "migrations", "001_init.sql"),
		filepath.Join("..", "..", "migrations", "002_users_mail_login.sql"),
		filepath.Join("..", "..", "migrations", "003_cleanup_rejected_users.sql"),
		filepath.Join("..", "..", "migrations", "004_cleanup_rejected_users_casefold.sql"),
		filepath.Join("..", "..", "migrations", "005_admin_query_indexes.sql"),
		filepath.Join("..", "..", "migrations", "006_users_recovery_email.sql"),
	} {
		if err := ApplyMigrationFile(sqdb, migration); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}

	for _, col := range []string{"mail_login", "recovery_email", "provision_state", "provision_error"} {
		if !hasColumn(t, sqdb, "users", col) {
			t.Fatalf("expected users.%s to exist after migration", col)
		}
	}
	if !hasColumn(t, sqdb, "sessions", "mail_secret") {
		t.Fatalf("expected sessions.mail_secret to exist after migration")
	}

	now := time.Now().UTC()
	if _, err := sqdb.Exec(
		`INSERT INTO users(id,email,password_hash,role,status,created_at) VALUES(?,?,?,?,?,?)`,
		"u1", "legacy@example.com", "legacy_hash", "admin", "active", now,
	); err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}

	st := store.New(sqdb)
	u, err := st.GetUserByEmail(context.Background(), "legacy@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail should work after compatibility migration, got: %v", err)
	}
	if u.ProvisionState != "pending" {
		t.Fatalf("expected default provision_state pending, got %q", u.ProvisionState)
	}
}

func TestSortedMigrationFilesIncludesLatestSchemaUpdates(t *testing.T) {
	files, err := SortedMigrationFiles(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("sorted migration files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected migrations to exist")
	}
	foundProvidersMigration := false
	foundLocalePreferenceMigration := false
	foundFormatLocalePreferenceMigration := false
	foundOutboundCampaignMigration := false
	foundReplyOpsSuppressionMigration := false
	foundOutboundIntelligenceMigration := false
	for i, file := range files {
		if i > 0 && filepath.Base(files[i-1]) > filepath.Base(file) {
			t.Fatalf("expected migrations to be sorted, got %s before %s", filepath.Base(files[i-1]), filepath.Base(file))
		}
		if filepath.Base(file) == "032_mail_account_providers.sql" {
			foundProvidersMigration = true
		}
		if filepath.Base(file) == "033_user_preferences_locale.sql" {
			foundLocalePreferenceMigration = true
		}
		if filepath.Base(file) == "034_user_preferences_format_locale.sql" {
			foundFormatLocalePreferenceMigration = true
		}
		if filepath.Base(file) == "035_outbound_campaigns.sql" {
			foundOutboundCampaignMigration = true
		}
		if filepath.Base(file) == "041_outbound_suppressions.sql" {
			foundReplyOpsSuppressionMigration = true
		}
		if filepath.Base(file) == "042_outbound_campaign_intelligence.sql" {
			foundOutboundIntelligenceMigration = true
		}
	}
	if !foundProvidersMigration {
		t.Fatal("expected migration list to include 032_mail_account_providers.sql")
	}
	if !foundLocalePreferenceMigration {
		t.Fatal("expected migration list to include 033_user_preferences_locale.sql")
	}
	if !foundFormatLocalePreferenceMigration {
		t.Fatal("expected migration list to include 034_user_preferences_format_locale.sql")
	}
	if !foundOutboundCampaignMigration {
		t.Fatal("expected migration list to include 035_outbound_campaigns.sql")
	}
	if !foundReplyOpsSuppressionMigration {
		t.Fatal("expected migration list to include 041_outbound_suppressions.sql")
	}
	if !foundOutboundIntelligenceMigration {
		t.Fatal("expected migration list to include 042_outbound_campaign_intelligence.sql")
	}
	if _, err := os.Stat(files[0]); err != nil {
		t.Fatalf("expected first migration file to exist: %v", err)
	}
}

func hasColumn(t *testing.T, sqdb *sql.DB, tableName, colName string) bool {
	t.Helper()
	rows, err := sqdb.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		t.Fatalf("table_info %s: %v", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info %s: %v", tableName, err)
		}
		if name == colName {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info %s: %v", tableName, err)
	}
	return false
}

func TestCleanupRejectedUsersMigrationRemovesLegacyRows(t *testing.T) {
	sqdb, err := OpenSQLite(filepath.Join(t.TempDir(), "cleanup.db"), 1, 1, time.Minute)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqdb.Close() })

	if err := ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "001_init.sql")); err != nil {
		t.Fatalf("apply migration 001: %v", err)
	}
	if err := ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "002_users_mail_login.sql")); err != nil {
		t.Fatalf("apply migration 002: %v", err)
	}

	now := time.Now().UTC()
	_, err = sqdb.Exec(
		`INSERT INTO users(id,email,password_hash,role,status,provision_state,created_at) VALUES(?,?,?,?,?,?,?)`,
		"u_rej", "rejected@example.com", "hash", "user", "rejected", "pending", now,
	)
	if err != nil {
		t.Fatalf("insert rejected user: %v", err)
	}
	_, err = sqdb.Exec(
		`INSERT INTO sessions(id,user_id,token_hash,mail_secret,expires_at,idle_expires_at,created_at,last_seen_at) VALUES(?,?,?,?,?,?,?,?)`,
		"s_rej", "u_rej", "token_hash_rej", "", now.Add(time.Hour), now.Add(time.Hour), now, now,
	)
	if err != nil {
		t.Fatalf("insert rejected user session: %v", err)
	}
	_, err = sqdb.Exec(
		`INSERT INTO password_reset_tokens(id,user_id,token_hash,expires_at,created_at) VALUES(?,?,?,?,?)`,
		"p_rej", "u_rej", "reset_hash_rej", now.Add(time.Hour), now,
	)
	if err != nil {
		t.Fatalf("insert rejected reset token: %v", err)
	}

	if err := ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "003_cleanup_rejected_users.sql")); err != nil {
		t.Fatalf("apply migration 003: %v", err)
	}
	if err := ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "004_cleanup_rejected_users_casefold.sql")); err != nil {
		t.Fatalf("apply migration 004: %v", err)
	}
	if err := ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "005_admin_query_indexes.sql")); err != nil {
		t.Fatalf("apply migration 004: %v", err)
	}
	if err := ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "006_users_recovery_email.sql")); err != nil {
		t.Fatalf("apply migration 006: %v", err)
	}

	var usersCount int
	if err := sqdb.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM users WHERE lower(trim(coalesce(status, '')))='rejected'`).Scan(&usersCount); err != nil {
		t.Fatalf("count rejected users: %v", err)
	}
	if usersCount != 0 {
		t.Fatalf("expected rejected users cleanup, found %d rows", usersCount)
	}

	var sessCount int
	if err := sqdb.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM sessions WHERE user_id='u_rej'`).Scan(&sessCount); err != nil {
		t.Fatalf("count rejected sessions: %v", err)
	}
	if sessCount != 0 {
		t.Fatalf("expected rejected sessions cleanup, found %d rows", sessCount)
	}

	var resetCount int
	if err := sqdb.QueryRowContext(context.Background(), `SELECT COUNT(1) FROM password_reset_tokens WHERE user_id='u_rej'`).Scan(&resetCount); err != nil {
		t.Fatalf("count rejected reset tokens: %v", err)
	}
	if resetCount != 0 {
		t.Fatalf("expected rejected reset token cleanup, found %d rows", resetCount)
	}
}

func TestMFAOnboardingMigrationBackfillsLegacyPromptOnlyForActiveUsersWithoutMFA(t *testing.T) {
	sqdb, err := OpenSQLite(filepath.Join(t.TempDir(), "mfa017.db"), 1, 1, time.Minute)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqdb.Close() })

	for _, migration := range []string{
		filepath.Join("..", "..", "migrations", "001_init.sql"),
		filepath.Join("..", "..", "migrations", "002_users_mail_login.sql"),
		filepath.Join("..", "..", "migrations", "003_cleanup_rejected_users.sql"),
		filepath.Join("..", "..", "migrations", "004_cleanup_rejected_users_casefold.sql"),
		filepath.Join("..", "..", "migrations", "005_admin_query_indexes.sql"),
		filepath.Join("..", "..", "migrations", "006_users_recovery_email.sql"),
		filepath.Join("..", "..", "migrations", "007_mail_accounts.sql"),
		filepath.Join("..", "..", "migrations", "008_mail_index.sql"),
		filepath.Join("..", "..", "migrations", "009_preferences_and_search.sql"),
		filepath.Join("..", "..", "migrations", "010_drafts_schedule.sql"),
		filepath.Join("..", "..", "migrations", "011_rules_sieve.sql"),
		filepath.Join("..", "..", "migrations", "012_mfa_totp_webauthn.sql"),
		filepath.Join("..", "..", "migrations", "013_crypto_keys.sql"),
		filepath.Join("..", "..", "migrations", "014_session_management.sql"),
		filepath.Join("..", "..", "migrations", "015_sync_state.sql"),
		filepath.Join("..", "..", "migrations", "016_quota_and_health.sql"),
	} {
		if err := ApplyMigrationFile(sqdb, migration); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}

	now := time.Now().UTC()
	_, err = sqdb.Exec(
		`INSERT INTO users(id,email,password_hash,role,status,provision_state,created_at,mfa_preference,legacy_mfa_prompt_pending)
		 VALUES
		 ('u_no_mfa','u_no_mfa@example.com','h','user','active','ok',?,'none',0),
		 ('u_totp','u_totp@example.com','h','user','active','ok',?,'totp',0),
		 ('u_webauthn','u_webauthn@example.com','h','user','active','ok',?,'webauthn',0),
		 ('u_suspended','u_suspended@example.com','h','user','suspended','ok',?,'none',0)`,
		now, now, now, now,
	)
	if err != nil {
		t.Fatalf("insert users: %v", err)
	}
	if _, err := sqdb.Exec(
		`INSERT INTO mfa_totp(user_id,secret_enc,issuer,account_name,enabled,enrolled_at,updated_at)
		 VALUES(?,?,?,?,?,?,?)`,
		"u_totp", "enc", "Despatch", "u_totp@example.com", 1, now, now,
	); err != nil {
		t.Fatalf("insert totp: %v", err)
	}
	if _, err := sqdb.Exec(
		`INSERT INTO mfa_webauthn_credentials(id,user_id,credential_id,public_key,sign_count,transports_json,name,created_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		"cred1", "u_webauthn", "cred-id-1", "pub", 1, "[]", "Passkey", now,
	); err != nil {
		t.Fatalf("insert webauthn: %v", err)
	}

	if err := ApplyMigrationFile(sqdb, filepath.Join("..", "..", "migrations", "017_mfa_onboarding_flags.sql")); err != nil {
		t.Fatalf("apply migration 017: %v", err)
	}

	for _, col := range []string{"mfa_preference", "legacy_mfa_prompt_pending", "mfa_setup_switch_used"} {
		if !hasColumn(t, sqdb, "users", col) {
			t.Fatalf("expected users.%s to exist after migration 017", col)
		}
	}
	if !hasColumn(t, sqdb, "registrations", "mfa_preference") {
		t.Fatalf("expected registrations.mfa_preference to exist after migration 017")
	}

	type expected struct {
		userID string
		want   int
	}
	for _, tc := range []expected{
		{userID: "u_no_mfa", want: 1},
		{userID: "u_totp", want: 0},
		{userID: "u_webauthn", want: 0},
		{userID: "u_suspended", want: 0},
	} {
		var got int
		if err := sqdb.QueryRow(`SELECT legacy_mfa_prompt_pending FROM users WHERE id=?`, tc.userID).Scan(&got); err != nil {
			t.Fatalf("read legacy prompt for %s: %v", tc.userID, err)
		}
		if got != tc.want {
			t.Fatalf("unexpected legacy prompt for %s: got=%d want=%d", tc.userID, got, tc.want)
		}
	}
}

func TestMFAUsabilityTrustedDevicesMigrationAddsBackupAndTrustedDeviceSchema(t *testing.T) {
	sqdb, err := OpenSQLite(filepath.Join(t.TempDir(), "mfa018.db"), 1, 1, time.Minute)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqdb.Close() })

	for _, migration := range []string{
		filepath.Join("..", "..", "migrations", "001_init.sql"),
		filepath.Join("..", "..", "migrations", "002_users_mail_login.sql"),
		filepath.Join("..", "..", "migrations", "003_cleanup_rejected_users.sql"),
		filepath.Join("..", "..", "migrations", "004_cleanup_rejected_users_casefold.sql"),
		filepath.Join("..", "..", "migrations", "005_admin_query_indexes.sql"),
		filepath.Join("..", "..", "migrations", "006_users_recovery_email.sql"),
		filepath.Join("..", "..", "migrations", "007_mail_accounts.sql"),
		filepath.Join("..", "..", "migrations", "008_mail_index.sql"),
		filepath.Join("..", "..", "migrations", "009_preferences_and_search.sql"),
		filepath.Join("..", "..", "migrations", "010_drafts_schedule.sql"),
		filepath.Join("..", "..", "migrations", "011_rules_sieve.sql"),
		filepath.Join("..", "..", "migrations", "012_mfa_totp_webauthn.sql"),
		filepath.Join("..", "..", "migrations", "013_crypto_keys.sql"),
		filepath.Join("..", "..", "migrations", "014_session_management.sql"),
		filepath.Join("..", "..", "migrations", "015_sync_state.sql"),
		filepath.Join("..", "..", "migrations", "016_quota_and_health.sql"),
		filepath.Join("..", "..", "migrations", "017_mfa_onboarding_flags.sql"),
		filepath.Join("..", "..", "migrations", "018_mfa_usability_trusted_devices.sql"),
		filepath.Join("..", "..", "migrations", "019_users_mail_secret.sql"),
		filepath.Join("..", "..", "migrations", "020_mail_index_scoped_ids.sql"),
	} {
		if err := ApplyMigrationFile(sqdb, migration); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}

	if !hasColumn(t, sqdb, "users", "mfa_backup_completed") {
		t.Fatalf("expected users.mfa_backup_completed to exist after migration 018")
	}
	if !hasColumn(t, sqdb, "mfa_trusted_devices", "token_hash") {
		t.Fatalf("expected mfa_trusted_devices.token_hash to exist after migration 018")
	}
	if !hasColumn(t, sqdb, "users", "mail_secret_enc") {
		t.Fatalf("expected users.mail_secret_enc to exist after migration 019")
	}
	if !hasColumn(t, sqdb, "users", "mail_secret_updated_at") {
		t.Fatalf("expected users.mail_secret_updated_at to exist after migration 019")
	}

	now := time.Now().UTC()
	_, err = sqdb.Exec(
		`INSERT INTO users(id,email,password_hash,role,status,provision_state,created_at,mfa_preference,legacy_mfa_prompt_pending,mfa_backup_completed)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		"u018", "u018@example.com", "h", "user", "active", "ok", now, "none", 0, 1,
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err = sqdb.Exec(
		`INSERT INTO mfa_trusted_devices(id,user_id,token_hash,ua_hash,ip_hint,device_label,created_at,expires_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		"td018", "u018", "hash018", "ua018", "127.0.0.1", "Laptop", now, now.Add(30*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("insert trusted device: %v", err)
	}
}
