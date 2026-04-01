DROP INDEX IF EXISTS idx_webhook_deliveries_subscription_id;
ALTER TABLE webhook_deliveries DROP COLUMN IF EXISTS subscription_id;
ALTER TABLE webhook_subscriptions DROP COLUMN IF EXISTS secret_grace_expires_at;
ALTER TABLE webhook_subscriptions DROP COLUMN IF EXISTS previous_secret;
