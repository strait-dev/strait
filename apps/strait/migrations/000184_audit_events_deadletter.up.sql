-- audit_events_deadletter stores audit events that failed to write to the
-- primary audit_events table after exhausting in-memory retries. Spilling
-- here is a reliability control: the event survives process restart and
-- can be replayed by a future reclaimer. No FK to audit_events — these
-- are events that never successfully reached the main table, so they are
-- not part of the HMAC chain and do not participate in chain verification.
CREATE TABLE IF NOT EXISTS audit_events_deadletter (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    actor_id      TEXT NOT NULL DEFAULT '',
    actor_type    TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT NOT NULL DEFAULT '',
    details       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    queued_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error    TEXT NOT NULL DEFAULT '',
    retry_count   INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_audit_events_deadletter_project_queued
    ON audit_events_deadletter(project_id, queued_at ASC);
