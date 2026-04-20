-- Track immutable retry lineage for quarantined enqueue_outbox rows.
-- Retry actions clone a fresh row and point it back at the quarantined
-- source via retry_of_outbox_id; no foreign key is added so purge can
-- hard-delete the source row without referential cleanup.
ALTER TABLE enqueue_outbox
    ADD COLUMN IF NOT EXISTS retry_of_outbox_id TEXT NULL;

UPDATE schema_version SET version = 215, updated_at = NOW();
