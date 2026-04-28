CREATE TABLE IF NOT EXISTS recipient_state (
  user_id TEXT NOT NULL,
  recipient_email TEXT NOT NULL COLLATE NOCASE,
  primary_contact_id TEXT,
  recipient_domain TEXT NOT NULL COLLATE NOCASE,
  status TEXT NOT NULL DEFAULT 'active',
  scope TEXT NOT NULL DEFAULT 'workspace',
  last_reply_at DATETIME,
  last_reply_outcome TEXT NOT NULL DEFAULT '',
  suppressed_until DATETIME,
  suppression_reason TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  updated_at DATETIME NOT NULL,
  PRIMARY KEY(user_id, recipient_email),
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY(primary_contact_id) REFERENCES contacts(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_recipient_state_domain
  ON recipient_state(user_id, recipient_domain, status, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_recipient_state_status
  ON recipient_state(user_id, status, updated_at DESC);
