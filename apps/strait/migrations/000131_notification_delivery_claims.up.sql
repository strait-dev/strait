ALTER TABLE notification_deliveries
    ADD COLUMN claim_token TEXT,
    ADD COLUMN lease_expires_at TIMESTAMPTZ;

CREATE INDEX idx_notification_deliveries_processing_lease
    ON notification_deliveries(status, lease_expires_at)
    WHERE status = 'processing';
