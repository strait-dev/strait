DROP INDEX IF EXISTS idx_audit_events_request_id;

ALTER TABLE audit_events_deadletter
    DROP COLUMN IF EXISTS schema_version,
    DROP COLUMN IF EXISTS trace_id,
    DROP COLUMN IF EXISTS request_id,
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS remote_ip;

ALTER TABLE audit_events
    DROP COLUMN IF EXISTS schema_version,
    DROP COLUMN IF EXISTS trace_id,
    DROP COLUMN IF EXISTS request_id,
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS remote_ip;
