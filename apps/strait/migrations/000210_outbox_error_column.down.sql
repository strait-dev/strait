ALTER TABLE enqueue_outbox DROP COLUMN IF EXISTS error;
UPDATE schema_version SET version = 209, updated_at = NOW();
