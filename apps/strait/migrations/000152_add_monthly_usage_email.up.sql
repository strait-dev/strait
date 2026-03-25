ALTER TABLE organization_subscriptions
  ADD COLUMN IF NOT EXISTS monthly_usage_email BOOLEAN NOT NULL DEFAULT true;
