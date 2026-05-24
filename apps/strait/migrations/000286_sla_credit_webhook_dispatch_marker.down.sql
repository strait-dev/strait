DROP INDEX IF EXISTS idx_sla_credits_pending_webhook_dispatch;

ALTER TABLE sla_credits
    DROP COLUMN IF EXISTS webhook_dispatched_at;
