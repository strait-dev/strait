ALTER TABLE organization_subscriptions
  DROP COLUMN IF EXISTS override_daily_run_limit,
  DROP COLUMN IF EXISTS override_concurrent_run_limit,
  DROP COLUMN IF EXISTS enforcement_mode;
