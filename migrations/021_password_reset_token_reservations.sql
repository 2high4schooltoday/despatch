ALTER TABLE password_reset_tokens ADD COLUMN reserved_at DATETIME;
ALTER TABLE password_reset_tokens ADD COLUMN reservation_id TEXT;

CREATE INDEX IF NOT EXISTS idx_password_reset_reserved_at ON password_reset_tokens(reserved_at);
