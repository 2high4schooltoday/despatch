CREATE TABLE IF NOT EXISTS mail_triage_categories (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  name TEXT NOT NULL,
  name_norm TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  UNIQUE(user_id, name_norm),
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mail_triage_categories_user_updated
  ON mail_triage_categories(user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS mail_triage_tags (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  name TEXT NOT NULL,
  name_norm TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  UNIQUE(user_id, name_norm),
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mail_triage_tags_user_updated
  ON mail_triage_tags(user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS mail_thread_triage (
  triage_key TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  source TEXT NOT NULL,
  account_id TEXT NOT NULL DEFAULT '',
  thread_id TEXT NOT NULL,
  mailbox_name TEXT NOT NULL DEFAULT '',
  subject TEXT NOT NULL DEFAULT '',
  from_value TEXT NOT NULL DEFAULT '',
  category_id TEXT,
  snoozed_until TIMESTAMP,
  reminder_at TIMESTAMP,
  last_reminder_notified_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  UNIQUE(user_id, source, account_id, thread_id),
  FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
  FOREIGN KEY(category_id) REFERENCES mail_triage_categories(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_mail_thread_triage_lookup
  ON mail_thread_triage(user_id, source, account_id, thread_id);

CREATE INDEX IF NOT EXISTS idx_mail_thread_triage_snooze
  ON mail_thread_triage(user_id, snoozed_until);

CREATE INDEX IF NOT EXISTS idx_mail_thread_triage_reminder
  ON mail_thread_triage(user_id, reminder_at, last_reminder_notified_at);

CREATE TABLE IF NOT EXISTS mail_thread_triage_tags (
  triage_key TEXT NOT NULL,
  tag_id TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL,
  PRIMARY KEY(triage_key, tag_id),
  FOREIGN KEY(triage_key) REFERENCES mail_thread_triage(triage_key) ON DELETE CASCADE,
  FOREIGN KEY(tag_id) REFERENCES mail_triage_tags(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_mail_thread_triage_tags_tag
  ON mail_thread_triage_tags(tag_id, triage_key);
