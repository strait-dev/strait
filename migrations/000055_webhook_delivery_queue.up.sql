-- Extend webhook_deliveries to support event trigger notifications.
-- The existing table tracks job run webhook deliveries (run_id + job_id).
-- Event trigger deliveries use event_trigger_id instead.
ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS event_trigger_id TEXT REFERENCES event_triggers(id) ON DELETE CASCADE;
ALTER TABLE webhook_deliveries ALTER COLUMN run_id DROP NOT NULL;
ALTER TABLE webhook_deliveries ALTER COLUMN job_id DROP NOT NULL;

-- Partial index for the delivery worker: pending items ready for attempt.
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending_retry
    ON webhook_deliveries (next_retry_at)
    WHERE status = 'pending' AND next_retry_at IS NOT NULL;
