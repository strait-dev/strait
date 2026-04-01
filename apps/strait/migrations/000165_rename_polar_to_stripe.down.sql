-- Revert Stripe columns back to Polar in organization_subscriptions.
ALTER TABLE organization_subscriptions RENAME COLUMN stripe_subscription_id TO polar_subscription_id;
ALTER TABLE organization_subscriptions RENAME COLUMN stripe_customer_id TO polar_customer_id;

-- Recreate indexes with old names.
DROP INDEX IF EXISTS idx_org_subscriptions_stripe_sub_id;
DROP INDEX IF EXISTS idx_org_subscriptions_stripe_cust_id;
CREATE INDEX idx_org_subscriptions_polar_sub_id ON organization_subscriptions(polar_subscription_id);
CREATE INDEX idx_org_subscriptions_polar_cust_id ON organization_subscriptions(polar_customer_id);

-- Revert Stripe column in organization_addons.
ALTER TABLE organization_addons RENAME COLUMN stripe_subscription_id TO polar_subscription_id;

-- Recreate unique index with old column name.
DROP INDEX IF EXISTS idx_org_addons_unique;
CREATE UNIQUE INDEX idx_org_addons_unique
    ON organization_addons (org_id, addon_type, polar_subscription_id)
    WHERE active = true;
