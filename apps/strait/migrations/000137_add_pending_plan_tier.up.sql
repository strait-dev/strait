-- Ensure organization_subscriptions table exists (created in migration 148 on this branch,
-- but ALTERed by earlier master migrations 137-145).
CREATE TABLE IF NOT EXISTS organization_subscriptions (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    org_id TEXT NOT NULL UNIQUE,
    plan_tier TEXT NOT NULL DEFAULT 'free',
    polar_subscription_id TEXT,
    polar_customer_id TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    current_period_start TIMESTAMPTZ,
    current_period_end TIMESTAMPTZ,
    spending_limit_microusd BIGINT DEFAULT -1,
    limit_action TEXT DEFAULT 'reject',
    canceled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_org_subscriptions_polar_sub_id ON organization_subscriptions(polar_subscription_id);
CREATE INDEX IF NOT EXISTS idx_org_subscriptions_polar_cust_id ON organization_subscriptions(polar_customer_id);

ALTER TABLE organization_subscriptions ADD COLUMN IF NOT EXISTS pending_plan_tier TEXT;
