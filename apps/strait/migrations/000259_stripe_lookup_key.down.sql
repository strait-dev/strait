ALTER TABLE organization_subscriptions DROP COLUMN IF EXISTS stripe_lookup_key;
ALTER TABLE organization_addons DROP COLUMN IF EXISTS stripe_lookup_key;
