DROP INDEX IF EXISTS idx_org_subscriptions_stripe_sub_id;
DROP INDEX IF EXISTS idx_org_subscriptions_stripe_cust_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_org_subscriptions_stripe_sub_id
    ON organization_subscriptions(stripe_subscription_id)
    WHERE stripe_subscription_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_org_subscriptions_stripe_cust_id
    ON organization_subscriptions(stripe_customer_id)
    WHERE stripe_customer_id IS NOT NULL;
