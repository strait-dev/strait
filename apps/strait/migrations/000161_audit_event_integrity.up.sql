ALTER TABLE audit_events
    ADD COLUMN IF NOT EXISTS signature TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS previous_hash TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_audit_events_project_integrity
    ON audit_events(project_id, created_at ASC);
