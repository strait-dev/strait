ALTER TABLE audit_events
    ADD COLUMN IF NOT EXISTS resource_type TEXT,
    ADD COLUMN IF NOT EXISTS resource_id TEXT,
    ADD COLUMN IF NOT EXISTS changes JSONB,
    ADD COLUMN IF NOT EXISTS ip_address TEXT,
    ADD COLUMN IF NOT EXISTS user_agent TEXT;

CREATE INDEX IF NOT EXISTS idx_audit_events_resource_lookup
    ON audit_events (resource_type, resource_id);
