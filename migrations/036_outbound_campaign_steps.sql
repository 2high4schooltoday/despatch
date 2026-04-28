CREATE TABLE IF NOT EXISTS outbound_campaign_steps (
  id TEXT PRIMARY KEY,
  campaign_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  kind TEXT NOT NULL DEFAULT 'email',
  thread_mode TEXT NOT NULL DEFAULT 'same_thread',
  subject_template TEXT NOT NULL DEFAULT '',
  body_template TEXT NOT NULL DEFAULT '',
  wait_interval_minutes INTEGER NOT NULL DEFAULT 0,
  send_window_json TEXT NOT NULL DEFAULT '{}',
  stop_if_replied INTEGER NOT NULL DEFAULT 1,
  stop_if_clicked INTEGER NOT NULL DEFAULT 0,
  stop_if_booked INTEGER NOT NULL DEFAULT 0,
  stop_if_unsubscribed INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE(campaign_id, position),
  FOREIGN KEY(campaign_id) REFERENCES outbound_campaigns(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_outbound_campaign_steps_campaign_position
  ON outbound_campaign_steps(campaign_id, position ASC);
