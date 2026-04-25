-- R3 Phase 8: transactional outbox.
--
-- Callers that commit a user-facing DB transaction and then call
-- queue.Enqueue separately have a reliability gap: a crash between
-- commit and enqueue silently drops the intent. The outbox lets
-- callers write the enqueue intent *inside* their own transaction
-- via WriteOutboxInTx; a background flusher promotes outbox rows
-- into job_runs using FOR UPDATE SKIP LOCKED so multiple flushers
-- are safe by construction.

CREATE TABLE IF NOT EXISTS enqueue_outbox (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL,
    job_id           TEXT NOT NULL,
    payload          JSONB,
    metadata         JSONB NOT NULL DEFAULT '{}',
    idempotency_key  TEXT,
    scheduled_at     TIMESTAMPTZ,
    priority         INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    consumed_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_enqueue_outbox_unconsumed
    ON enqueue_outbox (created_at)
    WHERE consumed_at IS NULL;
