ALTER TABLE organization_subscriptions
  ADD COLUMN IF NOT EXISTS override_daily_run_limit INT DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS override_concurrent_run_limit INT DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS enforcement_mode TEXT NOT NULL DEFAULT 'enforce';
