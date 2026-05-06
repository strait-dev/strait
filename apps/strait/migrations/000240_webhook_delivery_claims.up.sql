ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS claim_token TEXT,
    ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;
