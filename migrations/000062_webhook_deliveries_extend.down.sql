-- 000062: Revert webhook_deliveries extension
DROP INDEX IF EXISTS idx_webhook_deliveries_url_dead;
DROP INDEX IF EXISTS idx_webhook_deliveries_status_created;
DROP INDEX IF EXISTS idx_webhook_deliveries_run_seq;
DROP INDEX IF EXISTS idx_webhook_deliveries_queue;

ALTER TABLE webhook_deliveries
  DROP COLUMN IF EXISTS last_attempt_at,
  DROP COLUMN IF EXISTS sequence,
  DROP COLUMN IF EXISTS response_time_ms,
  DROP COLUMN IF EXISTS last_response_body,
  DROP COLUMN IF EXISTS payload_size_bytes,
  DROP COLUMN IF EXISTS event_type,
  DROP COLUMN IF EXISTS payload,
  DROP COLUMN IF EXISTS webhook_secret_prev,
  DROP COLUMN IF EXISTS webhook_secret,
  DROP COLUMN IF EXISTS subscription_id,
  DROP COLUMN IF EXISTS workflow_run_id;
