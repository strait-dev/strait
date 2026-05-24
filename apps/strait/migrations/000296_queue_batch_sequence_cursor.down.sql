INSERT INTO queue_batches (id, sealed_until)
SELECT DISTINCT batch_id, NOW()
FROM queue_entries
WHERE batch_id IS NOT NULL
ON CONFLICT (id) DO NOTHING;

ALTER TABLE queue_entries
    DROP CONSTRAINT IF EXISTS queue_entries_batch_id_fkey;

ALTER TABLE queue_entries
    ADD CONSTRAINT queue_entries_batch_id_fkey
    FOREIGN KEY (batch_id) REFERENCES queue_batches(id) ON DELETE SET NULL;

DROP SEQUENCE IF EXISTS queue_batch_id_seq;
