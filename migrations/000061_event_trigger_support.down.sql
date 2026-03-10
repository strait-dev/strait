DROP INDEX IF EXISTS idx_webhook_deliveries_pending_retry;

-- Remove event-trigger-only deliveries before restoring NOT NULL constraints.
DELETE FROM webhook_deliveries WHERE event_trigger_id IS NOT NULL AND run_id IS NULL;
ALTER TABLE webhook_deliveries DROP COLUMN IF EXISTS event_trigger_id;
ALTER TABLE webhook_deliveries ALTER COLUMN run_id SET NOT NULL;
ALTER TABLE webhook_deliveries ALTER COLUMN job_id SET NOT NULL;

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
