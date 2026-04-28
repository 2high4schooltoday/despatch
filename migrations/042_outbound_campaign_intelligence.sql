ALTER TABLE outbound_campaigns
  ADD COLUMN goal_kind TEXT NOT NULL DEFAULT 'general_outreach';

ALTER TABLE outbound_campaigns
  ADD COLUMN playbook_key TEXT NOT NULL DEFAULT '';

ALTER TABLE outbound_campaigns
  ADD COLUMN campaign_mode TEXT NOT NULL DEFAULT 'new_threads';

ALTER TABLE outbound_campaigns
  ADD COLUMN governance_policy_json TEXT NOT NULL DEFAULT '{}';

ALTER TABLE outbound_campaign_steps
  ADD COLUMN task_policy_json TEXT NOT NULL DEFAULT '{}';

ALTER TABLE outbound_campaign_steps
  ADD COLUMN branch_policy_json TEXT NOT NULL DEFAULT '{}';

ALTER TABLE outbound_enrollments
  ADD COLUMN seed_context_json TEXT NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_outbound_campaigns_user_playbook
  ON outbound_campaigns(user_id, playbook_key, updated_at DESC);
