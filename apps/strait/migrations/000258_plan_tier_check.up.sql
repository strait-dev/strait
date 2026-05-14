-- Strict CHECK constraints on plan_tier columns to match the canonical
-- 6-tier model (free, starter, pro, scale, business, enterprise).
-- Wave 1 introduces the new business tier; this migration locks the set.

ALTER TABLE organization_subscriptions
    ADD CONSTRAINT organization_subscriptions_plan_tier_check
    CHECK (plan_tier IN ('free', 'starter', 'pro', 'scale', 'business', 'enterprise'));

ALTER TABLE organization_subscriptions
    ADD CONSTRAINT organization_subscriptions_pending_plan_tier_check
    CHECK (pending_plan_tier IS NULL OR pending_plan_tier IN ('free', 'starter', 'pro', 'scale', 'business', 'enterprise'));

ALTER TABLE project_quotas
    ADD CONSTRAINT project_quotas_plan_tier_check
    CHECK (plan_tier IS NULL OR plan_tier IN ('free', 'starter', 'pro', 'scale', 'business', 'enterprise'));
