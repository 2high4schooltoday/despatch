ALTER TABLE user_preferences ADD COLUMN default_sender_id TEXT NOT NULL DEFAULT '';
ALTER TABLE drafts ADD COLUMN sender_profile_id TEXT NOT NULL DEFAULT '';
