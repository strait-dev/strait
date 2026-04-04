-- 000164: Webhook signing key rotation with grace period.
-- Adds previous_secret and grace expiry to webhook_subscriptions for
-- dual-signature delivery during key rotation.
-- Adds subscription_id FK to webhook_deliveries so the delivery worker
-- can look up the signing secret at delivery time.

ALTER TABLE webhook_subscriptions
  ADD COLUMN IF NOT EXISTS previous_secret TEXT,
  ADD COLUMN IF NOT EXISTS secret_grace_expires_at TIMESTAMPTZ;

ALTER TABLE webhook_deliveries
  ADD COLUMN IF NOT EXISTS subscription_id TEXT REFERENCES webhook_subscriptions(id);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_subscription_id
  ON webhook_deliveries (subscription_id)
  WHERE subscription_id IS NOT NULL;
