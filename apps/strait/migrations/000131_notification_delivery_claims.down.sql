DROP INDEX IF EXISTS idx_notification_deliveries_processing_lease;

ALTER TABLE notification_deliveries
    DROP COLUMN IF EXISTS lease_expires_at,
    DROP COLUMN IF EXISTS claim_token;
