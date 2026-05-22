-- Partial index for env-scoped lookups by status/expiry. Mirrors the
-- existing idx_event_triggers_project pattern but adds the env axis.
-- CONCURRENTLY so the index can be built without an ACCESS EXCLUSIVE
-- lock on a hot table; cannot run inside a transaction (golang-migrate
-- handles this automatically when no other statements share the file).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_event_triggers_project_env_status
    ON event_triggers(project_id, environment_id, status);
