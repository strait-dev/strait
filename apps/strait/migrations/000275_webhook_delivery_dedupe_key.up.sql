ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS dedupe_key TEXT;
