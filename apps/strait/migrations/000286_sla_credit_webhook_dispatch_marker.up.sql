ALTER TABLE sla_credits
    ADD COLUMN IF NOT EXISTS webhook_dispatched_at TIMESTAMPTZ;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sla_credits_pending_webhook_dispatch
    ON sla_credits (org_id, period_start)
    WHERE webhook_dispatched_at IS NULL;
