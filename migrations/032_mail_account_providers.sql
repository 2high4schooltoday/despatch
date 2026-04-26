ALTER TABLE mail_accounts ADD COLUMN provider_type TEXT NOT NULL DEFAULT 'generic';
ALTER TABLE mail_accounts ADD COLUMN provider_label TEXT NOT NULL DEFAULT '';
ALTER TABLE mail_accounts ADD COLUMN auth_kind TEXT NOT NULL DEFAULT 'password';
ALTER TABLE mail_accounts ADD COLUMN connection_mode TEXT NOT NULL DEFAULT 'imap_smtp';
ALTER TABLE mail_accounts ADD COLUMN validation_imap_ready INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mail_accounts ADD COLUMN validation_smtp_ready INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mail_accounts ADD COLUMN validation_app_password_required INTEGER NOT NULL DEFAULT 0;
ALTER TABLE mail_accounts ADD COLUMN validation_error TEXT NOT NULL DEFAULT '';
ALTER TABLE mail_accounts ADD COLUMN last_validated_at DATETIME;

CREATE INDEX IF NOT EXISTS idx_mail_accounts_provider ON mail_accounts(user_id, provider_type);
