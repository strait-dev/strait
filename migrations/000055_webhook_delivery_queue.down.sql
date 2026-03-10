DROP INDEX IF EXISTS idx_webhook_deliveries_pending_retry;
DELETE FROM webhook_deliveries WHERE event_trigger_id IS NOT NULL;
ALTER TABLE webhook_deliveries DROP COLUMN IF EXISTS event_trigger_id;
ALTER TABLE webhook_deliveries ALTER COLUMN run_id SET NOT NULL;
ALTER TABLE webhook_deliveries ALTER COLUMN job_id SET NOT NULL;
