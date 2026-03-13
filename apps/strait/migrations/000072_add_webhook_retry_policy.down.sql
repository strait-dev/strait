-- Remove webhook_retry_policy column from webhook_deliveries.
ALTER TABLE webhook_deliveries
  DROP COLUMN IF EXISTS webhook_retry_policy;
