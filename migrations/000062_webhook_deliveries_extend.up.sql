-- 000062: Extend webhook_deliveries table for unified delivery system (Plan 2.1)
-- The webhook_deliveries table already exists (migration 000009, extended 000061).
-- This adds missing columns for the unified delivery worker.

ALTER TABLE webhook_deliveries
  ADD COLUMN IF NOT EXISTS workflow_run_id TEXT REFERENCES workflow_runs(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS subscription_id TEXT,
  ADD COLUMN IF NOT EXISTS webhook_secret TEXT,
  ADD COLUMN IF NOT EXISTS webhook_secret_prev TEXT,
  ADD COLUMN IF NOT EXISTS payload JSONB,
  ADD COLUMN IF NOT EXISTS event_type TEXT,
  ADD COLUMN IF NOT EXISTS payload_size_bytes INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_response_body TEXT,
  ADD COLUMN IF NOT EXISTS response_time_ms INT,
  ADD COLUMN IF NOT EXISTS sequence INT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ;

-- Add missing indexes for the delivery worker
CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_queue
  ON webhook_deliveries (next_retry_at) WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_run_seq
  ON webhook_deliveries (run_id, sequence) WHERE run_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_status_created
  ON webhook_deliveries (status, created_at);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_url_dead
  ON webhook_deliveries (webhook_url, created_at) WHERE status = 'dead';
