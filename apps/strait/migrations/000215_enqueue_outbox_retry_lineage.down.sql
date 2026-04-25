ALTER TABLE enqueue_outbox
    DROP COLUMN IF EXISTS retry_of_outbox_id;

UPDATE schema_version SET version = 211, updated_at = NOW();
