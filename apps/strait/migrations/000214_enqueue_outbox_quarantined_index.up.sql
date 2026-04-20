-- Support read-only admin inspection of quarantined enqueue_outbox rows.
-- Build concurrently to avoid write blocking on enqueue_outbox.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_enqueue_outbox_quarantined
    ON enqueue_outbox (project_id, consumed_at DESC, id DESC)
    WHERE consumed_at IS NOT NULL
      AND error IS NOT NULL
      AND error <> '';
