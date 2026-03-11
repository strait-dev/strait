DROP INDEX IF EXISTS idx_audit_events_resource_lookup;

ALTER TABLE audit_events
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS ip_address,
    DROP COLUMN IF EXISTS changes;
