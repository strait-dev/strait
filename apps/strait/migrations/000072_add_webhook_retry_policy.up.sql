-- Add webhook_retry_policy column to webhook_deliveries.
-- This column was referenced by application code but was never added via migration.
-- Supports retry policy values: exponential, linear, fixed.
ALTER TABLE webhook_deliveries
  ADD COLUMN IF NOT EXISTS webhook_retry_policy TEXT;
