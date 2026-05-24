DROP INDEX IF EXISTS idx_org_subscriptions_stripe_sub_id;
DROP INDEX IF EXISTS idx_org_subscriptions_stripe_cust_id;

CREATE INDEX IF NOT EXISTS idx_org_subscriptions_stripe_sub_id
    ON organization_subscriptions(stripe_subscription_id);

CREATE INDEX IF NOT EXISTS idx_org_subscriptions_stripe_cust_id
    ON organization_subscriptions(stripe_customer_id);
