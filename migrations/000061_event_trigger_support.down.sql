DROP INDEX IF EXISTS idx_webhook_deliveries_pending_retry;
ALTER TABLE webhook_deliveries DROP COLUMN IF EXISTS event_trigger_id;

DROP TABLE IF EXISTS event_triggers;

ALTER TABLE workflow_steps
  DROP COLUMN IF EXISTS event_key,
  DROP COLUMN IF EXISTS event_timeout_secs,
  DROP COLUMN IF EXISTS event_notify_url,
  DROP COLUMN IF EXISTS event_emit_key,
  DROP COLUMN IF EXISTS sleep_duration_secs;

ALTER TABLE workflow_version_steps
  DROP COLUMN IF EXISTS event_key,
  DROP COLUMN IF EXISTS event_timeout_secs,
  DROP COLUMN IF EXISTS event_notify_url,
  DROP COLUMN IF EXISTS event_emit_key,
  DROP COLUMN IF EXISTS sleep_duration_secs;
