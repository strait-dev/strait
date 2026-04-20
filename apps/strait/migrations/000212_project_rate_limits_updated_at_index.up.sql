-- Supports the backpressure sampler's ORDER BY updated_at DESC NULLS LAST
-- LIMIT N scan in queue.SampleAvailableTokens. Without this index the
-- sampler degrades to a sort over project_rate_limits every tick.
-- CONCURRENTLY so the index can be built without an ACCESS EXCLUSIVE
-- lock on a hot table; cannot run inside a transaction (golang-migrate
-- handles this automatically when no other statements share the file).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_project_rate_limits_updated_at_desc
    ON project_rate_limits (updated_at DESC NULLS LAST);
