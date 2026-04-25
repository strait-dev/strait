DROP INDEX IF EXISTS idx_audit_events_project_epoch_created;

ALTER TABLE audit_events_deadletter
    DROP COLUMN IF EXISTS rotation_epoch,
    DROP COLUMN IF EXISTS is_anchor;

ALTER TABLE audit_events
    DROP COLUMN IF EXISTS rotation_epoch,
    DROP COLUMN IF EXISTS is_anchor;
