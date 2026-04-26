CREATE TABLE IF NOT EXISTS reply_funnels (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  name TEXT NOT NULL,
  sender_name TEXT NOT NULL DEFAULT '',
  collector_account_id TEXT NOT NULL,
  reply_mode TEXT NOT NULL DEFAULT 'collector',
  routing_mode TEXT NOT NULL DEFAULT 'virtual_inbox',
  include_collector INTEGER NOT NULL DEFAULT 1,
  target_reply_count INTEGER NOT NULL DEFAULT 100,
  saved_search_id TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id),
  FOREIGN KEY(collector_account_id) REFERENCES mail_accounts(id)
);

CREATE INDEX IF NOT EXISTS idx_reply_funnels_user ON reply_funnels(user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_reply_funnels_collector ON reply_funnels(collector_account_id);

CREATE TABLE IF NOT EXISTS reply_funnel_accounts (
  id TEXT PRIMARY KEY,
  funnel_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'source',
  position INTEGER NOT NULL DEFAULT 0,
  sender_identity_id TEXT,
  redirect_rule_id TEXT,
  last_apply_error TEXT NOT NULL DEFAULT '',
  assisted_forwarding_state TEXT NOT NULL DEFAULT 'not_required',
  assisted_forwarding_notes TEXT NOT NULL DEFAULT '',
  assisted_forwarding_checked_at DATETIME,
  assisted_forwarding_confirmed_at DATETIME,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(funnel_id) REFERENCES reply_funnels(id) ON DELETE CASCADE,
  FOREIGN KEY(account_id) REFERENCES mail_accounts(id),
  FOREIGN KEY(sender_identity_id) REFERENCES mail_identities(id) ON DELETE SET NULL,
  FOREIGN KEY(redirect_rule_id) REFERENCES mail_rules(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_reply_funnel_accounts_unique ON reply_funnel_accounts(funnel_id, account_id, role);
CREATE INDEX IF NOT EXISTS idx_reply_funnel_accounts_funnel ON reply_funnel_accounts(funnel_id, role, position ASC);
CREATE INDEX IF NOT EXISTS idx_reply_funnel_accounts_account ON reply_funnel_accounts(account_id);
