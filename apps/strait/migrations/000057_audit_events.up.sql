CREATE TABLE IF NOT EXISTS audit_events (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    actor_id      TEXT,
    actor_type    TEXT,
    action        TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT,
    details       JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_events_project_created_at
    ON audit_events(project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_events_resource_created_at
    ON audit_events(resource_type, resource_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_events_actor_created_at
    ON audit_events(actor_id, created_at DESC);
