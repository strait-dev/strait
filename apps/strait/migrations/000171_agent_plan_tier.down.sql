ALTER TABLE org_subscriptions
    DROP COLUMN IF EXISTS agent_plan_tier,
    DROP COLUMN IF EXISTS agent_stripe_subscription_id,
    DROP COLUMN IF EXISTS agent_current_period_start,
    DROP COLUMN IF EXISTS agent_current_period_end,
    DROP COLUMN IF EXISTS agent_spending_limit_microusd,
    DROP COLUMN IF EXISTS agent_pending_plan_tier;
