DROP INDEX IF EXISTS idx_org_subscriptions_dunning_active;

ALTER TABLE organization_subscriptions
    DROP COLUMN IF EXISTS dunning_step,
    DROP COLUMN IF EXISTS dunning_entered_at,
    DROP COLUMN IF EXISTS dunning_last_tick_at,
    DROP COLUMN IF EXISTS dunning_resolved_at;
