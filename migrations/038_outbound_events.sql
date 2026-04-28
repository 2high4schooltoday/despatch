CREATE TABLE IF NOT EXISTS outbound_events (
  id TEXT PRIMARY KEY,
  campaign_id TEXT NOT NULL,
  enrollment_id TEXT NOT NULL,
  event_kind TEXT NOT NULL,
  event_payload_json TEXT NOT NULL DEFAULT '{}',
  actor_kind TEXT NOT NULL DEFAULT 'system',
  actor_ref TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  FOREIGN KEY(campaign_id) REFERENCES outbound_campaigns(id) ON DELETE CASCADE,
  FOREIGN KEY(enrollment_id) REFERENCES outbound_enrollments(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_outbound_events_campaign_created
  ON outbound_events(campaign_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_outbound_events_enrollment_created
  ON outbound_events(enrollment_id, created_at DESC);
