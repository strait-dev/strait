ALTER TABLE queue_entries
    DROP CONSTRAINT IF EXISTS queue_entries_batch_id_fkey;

CREATE SEQUENCE IF NOT EXISTS queue_batch_id_seq;

SELECT setval(
    'queue_batch_id_seq',
    GREATEST(
        COALESCE((SELECT MAX(batch_id) FROM queue_entries), 0),
        COALESCE((SELECT MAX(id) FROM queue_batches), 0),
        1
    ),
    true
);
