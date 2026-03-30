DROP INDEX IF EXISTS idx_audit_events_project_integrity;

ALTER TABLE audit_events
    DROP COLUMN IF EXISTS signature,
    DROP COLUMN IF EXISTS previous_hash;
