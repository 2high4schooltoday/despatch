CREATE TABLE IF NOT EXISTS outbound_enrollments (
  id TEXT PRIMARY KEY,
  campaign_id TEXT NOT NULL,
  contact_id TEXT,
  recipient_email TEXT NOT NULL COLLATE NOCASE,
  recipient_domain TEXT NOT NULL COLLATE NOCASE,
  sender_account_id TEXT,
  sender_profile_id TEXT,
  reply_funnel_id TEXT,
  thread_binding_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  current_step_position INTEGER NOT NULL DEFAULT 0,
  last_sent_message_id TEXT NOT NULL DEFAULT '',
  last_sent_at DATETIME,
  next_action_at DATETIME,
  pause_reason TEXT NOT NULL DEFAULT '',
  stop_reason TEXT NOT NULL DEFAULT '',
  reply_outcome TEXT NOT NULL DEFAULT '',
  reply_confidence REAL NOT NULL DEFAULT 0,
  manual_owner_user_id TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE(campaign_id, recipient_email),
  FOREIGN KEY(campaign_id) REFERENCES outbound_campaigns(id) ON DELETE CASCADE,
  FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE SET NULL,
  FOREIGN KEY(sender_account_id) REFERENCES mail_accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(reply_funnel_id) REFERENCES reply_funnels(id) ON DELETE SET NULL,
  FOREIGN KEY(manual_owner_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_outbound_enrollments_campaign_status_next
  ON outbound_enrollments(campaign_id, status, next_action_at);

CREATE INDEX IF NOT EXISTS idx_outbound_enrollments_status_next
  ON outbound_enrollments(status, next_action_at);

CREATE INDEX IF NOT EXISTS idx_outbound_enrollments_recipient_status
  ON outbound_enrollments(recipient_email, status);

CREATE INDEX IF NOT EXISTS idx_outbound_enrollments_domain_status
  ON outbound_enrollments(recipient_domain, status);

CREATE INDEX IF NOT EXISTS idx_outbound_enrollments_owner_status
  ON outbound_enrollments(manual_owner_user_id, status);
