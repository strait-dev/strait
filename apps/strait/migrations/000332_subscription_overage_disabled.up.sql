ALTER TABLE organization_subscriptions
    ADD COLUMN IF NOT EXISTS overage_disabled BOOLEAN;

UPDATE organization_subscriptions
SET overage_disabled = true
WHERE plan_tier = 'free';

UPDATE organization_subscriptions
SET overage_disabled = false
WHERE overage_disabled IS NULL;

ALTER TABLE organization_subscriptions
    ALTER COLUMN overage_disabled SET DEFAULT false;

COMMENT ON COLUMN organization_subscriptions.overage_disabled IS
    'When true, orgs are hard-capped at their included monthly run allowance instead of entering metered overage.';
