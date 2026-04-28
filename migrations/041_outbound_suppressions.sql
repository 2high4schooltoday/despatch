CREATE TABLE IF NOT EXISTS outbound_suppressions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  scope_kind TEXT NOT NULL,
  scope_value TEXT NOT NULL COLLATE NOCASE,
  campaign_id TEXT,
  reason TEXT NOT NULL DEFAULT '',
  source_kind TEXT NOT NULL DEFAULT 'manual',
  expires_at DATETIME,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY(campaign_id) REFERENCES outbound_campaigns(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_outbound_suppressions_scope_unique
  ON outbound_suppressions(user_id, scope_kind, scope_value, IFNULL(campaign_id, ''));

CREATE INDEX IF NOT EXISTS idx_outbound_suppressions_scope_lookup
  ON outbound_suppressions(user_id, scope_kind, scope_value, expires_at, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_outbound_suppressions_campaign
  ON outbound_suppressions(user_id, campaign_id, updated_at DESC);
