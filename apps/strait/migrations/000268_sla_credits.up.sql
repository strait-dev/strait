-- sla_credits is the durable record of every SLA-driven credit the platform
-- has issued to an enterprise customer. One row per (org, billing period).
-- The unique constraint is the idempotency key for the calculator tick: a
-- second tick in the same period observes the existing row and skips.
CREATE TABLE IF NOT EXISTS sla_credits (
    id                    UUID PRIMARY KEY,
    org_id                TEXT NOT NULL,
    period_start          TIMESTAMPTZ NOT NULL,
    period_end            TIMESTAMPTZ NOT NULL,
    uptime_pct            NUMERIC(6, 4) NOT NULL,
    target_pct            NUMERIC(6, 4) NOT NULL,
    credit_pct            SMALLINT NOT NULL,
    credit_microusd       BIGINT NOT NULL,
    stripe_credit_note_id TEXT,
    issued_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, period_start, period_end)
);

CREATE INDEX IF NOT EXISTS idx_sla_credits_org_period
    ON sla_credits (org_id, period_start DESC);
