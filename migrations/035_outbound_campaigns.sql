CREATE TABLE IF NOT EXISTS outbound_campaigns (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  name TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft',
  audience_source_kind TEXT NOT NULL DEFAULT 'manual',
  audience_source_ref TEXT NOT NULL DEFAULT '',
  sender_policy_kind TEXT NOT NULL DEFAULT 'preferred_sender',
  sender_policy_ref TEXT NOT NULL DEFAULT '',
  reply_policy_json TEXT NOT NULL DEFAULT '{}',
  suppression_policy_json TEXT NOT NULL DEFAULT '{}',
  schedule_policy_json TEXT NOT NULL DEFAULT '{}',
  compliance_policy_json TEXT NOT NULL DEFAULT '{}',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  launched_at DATETIME,
  completed_at DATETIME,
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_outbound_campaigns_user_updated
  ON outbound_campaigns(user_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_outbound_campaigns_user_status
  ON outbound_campaigns(user_id, status, updated_at DESC);
