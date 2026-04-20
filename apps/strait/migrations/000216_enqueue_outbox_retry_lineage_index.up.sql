-- Enforce at most one active retry clone per quarantined source row.
-- Build concurrently because enqueue_outbox is a hot write path and the
-- invariant only matters for unconsumed clones.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_enqueue_outbox_retry_of_active
    ON enqueue_outbox (retry_of_outbox_id)
    WHERE retry_of_outbox_id IS NOT NULL
      AND consumed_at IS NULL;
