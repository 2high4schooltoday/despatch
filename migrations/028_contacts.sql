CREATE TABLE IF NOT EXISTS contacts (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  notes TEXT NOT NULL DEFAULT '',
  preferred_account_id TEXT NOT NULL DEFAULT '',
  preferred_sender_id TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE INDEX IF NOT EXISTS idx_contacts_user_name ON contacts(user_id, name COLLATE NOCASE, updated_at DESC);

CREATE TABLE IF NOT EXISTS contact_emails (
  id TEXT PRIMARY KEY,
  contact_id TEXT NOT NULL,
  email TEXT NOT NULL COLLATE NOCASE,
  label TEXT NOT NULL DEFAULT '',
  is_primary INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_contact_emails_contact_email ON contact_emails(contact_id, email);
CREATE INDEX IF NOT EXISTS idx_contact_emails_email ON contact_emails(email, contact_id);

CREATE TABLE IF NOT EXISTS contact_nicknames (
  contact_id TEXT NOT NULL,
  nickname TEXT NOT NULL COLLATE NOCASE,
  created_at DATETIME NOT NULL,
  PRIMARY KEY(contact_id, nickname),
  FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_contact_nicknames_nickname ON contact_nicknames(nickname, contact_id);

CREATE TABLE IF NOT EXISTS contact_groups (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  name TEXT NOT NULL COLLATE NOCASE,
  description TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  FOREIGN KEY(user_id) REFERENCES users(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_contact_groups_user_name ON contact_groups(user_id, name);

CREATE TABLE IF NOT EXISTS contact_group_members (
  group_id TEXT NOT NULL,
  contact_id TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  PRIMARY KEY(group_id, contact_id),
  FOREIGN KEY(group_id) REFERENCES contact_groups(id) ON DELETE CASCADE,
  FOREIGN KEY(contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_contact_group_members_contact ON contact_group_members(contact_id, group_id);
