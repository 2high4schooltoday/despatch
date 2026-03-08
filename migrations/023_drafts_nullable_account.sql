PRAGMA foreign_keys=OFF;

DROP INDEX IF EXISTS idx_drafts_user;
DROP INDEX IF EXISTS idx_drafts_account;
DROP INDEX IF EXISTS idx_draft_versions_unique;
DROP INDEX IF EXISTS idx_draft_versions_created;
DROP INDEX IF EXISTS idx_scheduled_send_due;

ALTER TABLE drafts RENAME TO drafts_old;
ALTER TABLE draft_versions RENAME TO draft_versions_old;
ALTER TABLE scheduled_send_queue RENAME TO scheduled_send_queue_old;

CREATE TABLE drafts (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  account_id TEXT,
  identity_id TEXT NOT NULL DEFAULT '',
  to_value TEXT NOT NULL DEFAULT '',
  cc_value TEXT NOT NULL DEFAULT '',
  bcc_value TEXT NOT NULL DEFAULT '',
  subject TEXT NOT NULL DEFAULT '',
  body_text TEXT NOT NULL DEFAULT '',
  body_html TEXT NOT NULL DEFAULT '',
  attachments_json TEXT NOT NULL DEFAULT '[]',
  crypto_options_json TEXT NOT NULL DEFAULT '{}',
  send_mode TEXT NOT NULL DEFAULT 'manual',
  scheduled_for DATETIME,
  status TEXT NOT NULL DEFAULT 'draft',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  compose_mode TEXT NOT NULL DEFAULT 'send',
  context_message_id TEXT NOT NULL DEFAULT '',
  from_mode TEXT NOT NULL DEFAULT 'default',
  from_manual TEXT NOT NULL DEFAULT '',
  client_state_json TEXT NOT NULL DEFAULT '',
  FOREIGN KEY(user_id) REFERENCES users(id),
  FOREIGN KEY(account_id) REFERENCES mail_accounts(id)
);

CREATE TABLE draft_versions (
  id TEXT PRIMARY KEY,
  draft_id TEXT NOT NULL,
  version_no INTEGER NOT NULL,
  snapshot_json TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  FOREIGN KEY(draft_id) REFERENCES drafts(id)
);

CREATE TABLE scheduled_send_queue (
  id TEXT PRIMARY KEY,
  draft_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  due_at DATETIME NOT NULL,
  state TEXT NOT NULL DEFAULT 'queued',
  retry_count INTEGER NOT NULL DEFAULT 0,
  next_retry_at DATETIME,
  last_error TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(draft_id) REFERENCES drafts(id),
  FOREIGN KEY(user_id) REFERENCES users(id),
  FOREIGN KEY(account_id) REFERENCES mail_accounts(id)
);

INSERT INTO drafts(
  id,user_id,account_id,identity_id,to_value,cc_value,bcc_value,subject,body_text,body_html,
  attachments_json,crypto_options_json,send_mode,scheduled_for,status,created_at,updated_at,
  compose_mode,context_message_id,from_mode,from_manual,client_state_json
)
SELECT
  id,user_id,NULLIF(TRIM(account_id),''),identity_id,to_value,cc_value,bcc_value,subject,body_text,body_html,
  attachments_json,crypto_options_json,send_mode,scheduled_for,status,created_at,updated_at,
  compose_mode,context_message_id,from_mode,from_manual,client_state_json
FROM drafts_old;

INSERT INTO draft_versions(id,draft_id,version_no,snapshot_json,created_at)
SELECT id,draft_id,version_no,snapshot_json,created_at
FROM draft_versions_old;

INSERT INTO scheduled_send_queue(
  id,draft_id,user_id,account_id,due_at,state,retry_count,next_retry_at,last_error,created_at,updated_at
)
SELECT
  id,draft_id,user_id,account_id,due_at,state,retry_count,next_retry_at,last_error,created_at,updated_at
FROM scheduled_send_queue_old;

DROP TABLE scheduled_send_queue_old;
DROP TABLE draft_versions_old;
DROP TABLE drafts_old;

CREATE INDEX idx_drafts_user ON drafts(user_id, updated_at DESC);
CREATE INDEX idx_drafts_account ON drafts(account_id, updated_at DESC);
CREATE UNIQUE INDEX idx_draft_versions_unique ON draft_versions(draft_id, version_no);
CREATE INDEX idx_draft_versions_created ON draft_versions(draft_id, created_at DESC);
CREATE INDEX idx_scheduled_send_due ON scheduled_send_queue(state, due_at, next_retry_at);

PRAGMA foreign_keys=ON;
