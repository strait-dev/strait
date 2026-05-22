ALTER TABLE sla_credits
    ADD COLUMN IF NOT EXISTS webhook_dispatched_at TIMESTAMPTZ;

-- safety-ok: golang-migrate runs this multi-statement migration in a transaction;
-- CONCURRENTLY is not viable here, and this partial index covers pending credit webhooks.
CREATE INDEX IF NOT EXISTS idx_sla_credits_pending_webhook_dispatch
    ON sla_credits (org_id, period_start)
    WHERE webhook_dispatched_at IS NULL;
