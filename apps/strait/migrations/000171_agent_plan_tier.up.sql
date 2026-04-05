ALTER TABLE org_subscriptions
    ADD COLUMN IF NOT EXISTS agent_plan_tier TEXT NOT NULL DEFAULT 'agent_free',
    ADD COLUMN IF NOT EXISTS agent_stripe_subscription_id TEXT,
    ADD COLUMN IF NOT EXISTS agent_current_period_start TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS agent_current_period_end TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS agent_spending_limit_microusd BIGINT NOT NULL DEFAULT -1,
    ADD COLUMN IF NOT EXISTS agent_pending_plan_tier TEXT;
