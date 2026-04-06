CREATE TABLE IF NOT EXISTS notify_suppression_events (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id      TEXT NOT NULL,
    recipient_type  TEXT NOT NULL,
    recipient_id    TEXT NOT NULL,
    scope           TEXT NOT NULL DEFAULT 'global',
    channel         TEXT NOT NULL,
    action          TEXT NOT NULL,
    reason          TEXT,
    source          TEXT NOT NULL,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (action IN ('suppressed', 'unsuppressed'))
);

CREATE INDEX IF NOT EXISTS idx_notify_suppression_events_recipient
    ON notify_suppression_events (project_id, recipient_type, recipient_id, created_at DESC);
