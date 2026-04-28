CREATE TABLE IF NOT EXISTS mail_thread_bindings (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  thread_id TEXT NOT NULL DEFAULT '',
  binding_type TEXT NOT NULL DEFAULT 'campaign',
  campaign_id TEXT,
  enrollment_id TEXT,
  funnel_id TEXT,
  reply_account_id TEXT,
  reply_sender_profile_id TEXT,
  collector_account_id TEXT,
  owner_user_id TEXT,
  recipient_email TEXT NOT NULL DEFAULT '' COLLATE NOCASE,
  recipient_domain TEXT NOT NULL DEFAULT '' COLLATE NOCASE,
  root_outbound_message_id TEXT NOT NULL DEFAULT '',
  last_outbound_message_id TEXT NOT NULL DEFAULT '',
  last_reply_message_id TEXT NOT NULL DEFAULT '',
  thread_subject TEXT NOT NULL DEFAULT '',
  last_reply_at DATETIME,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(account_id) REFERENCES mail_accounts(id) ON DELETE CASCADE,
  FOREIGN KEY(campaign_id) REFERENCES outbound_campaigns(id) ON DELETE CASCADE,
  FOREIGN KEY(enrollment_id) REFERENCES outbound_enrollments(id) ON DELETE CASCADE,
  FOREIGN KEY(funnel_id) REFERENCES reply_funnels(id) ON DELETE SET NULL,
  FOREIGN KEY(reply_account_id) REFERENCES mail_accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(collector_account_id) REFERENCES mail_accounts(id) ON DELETE SET NULL,
  FOREIGN KEY(owner_user_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_mail_thread_bindings_account_thread
  ON mail_thread_bindings(account_id, thread_id);

CREATE INDEX IF NOT EXISTS idx_mail_thread_bindings_enrollment
  ON mail_thread_bindings(enrollment_id);

CREATE INDEX IF NOT EXISTS idx_mail_thread_bindings_campaign
  ON mail_thread_bindings(campaign_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_mail_thread_bindings_root_message
  ON mail_thread_bindings(root_outbound_message_id);

CREATE INDEX IF NOT EXISTS idx_mail_thread_bindings_last_message
  ON mail_thread_bindings(last_outbound_message_id);

CREATE INDEX IF NOT EXISTS idx_mail_thread_bindings_reply_message
  ON mail_thread_bindings(last_reply_message_id);
