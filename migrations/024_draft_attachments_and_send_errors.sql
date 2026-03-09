ALTER TABLE drafts ADD COLUMN last_send_error TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS draft_attachments (
  id TEXT PRIMARY KEY,
  draft_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  filename TEXT NOT NULL DEFAULT '',
  content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  inline_part INTEGER NOT NULL DEFAULT 0,
  content_id TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  data BLOB NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(draft_id) REFERENCES drafts(id),
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_draft_attachments_draft
ON draft_attachments(draft_id, sort_order, created_at);

CREATE INDEX IF NOT EXISTS idx_draft_attachments_user
ON draft_attachments(user_id, draft_id, created_at);
