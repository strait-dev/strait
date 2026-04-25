DROP INDEX IF EXISTS idx_audit_events_anchor_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_events_anchor_unique
    ON audit_events (project_id, rotation_epoch)
    WHERE is_anchor = TRUE;
