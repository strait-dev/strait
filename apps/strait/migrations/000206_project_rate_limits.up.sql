-- R3 Phase 7: per-project enqueue backpressure.
--
-- The API-layer rate limiter only catches HTTP callers. Internal
-- callers (workflow steps, cron triggers, retries, chaining) can
-- enqueue at unlimited rate and blow up the queue. project_rate_limits
-- is a DB-backed token bucket that queue.Enqueue/EnqueueBatch consults
-- when feature flag QUEUE_BACKPRESSURE_ENABLED is on.

CREATE TABLE IF NOT EXISTS project_rate_limits (
    project_id      TEXT PRIMARY KEY,
    tokens          INT NOT NULL DEFAULT 1000,
    max_tokens      INT NOT NULL DEFAULT 1000,
    refill_per_sec  INT NOT NULL DEFAULT 100,
    last_refill_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
