DROP INDEX IF EXISTS idx_audit_events_deadletter_project_created;
ALTER TABLE audit_events_deadletter
    DROP COLUMN IF EXISTS reclaimed_event_id,
    DROP COLUMN IF EXISTS attempt_count;
