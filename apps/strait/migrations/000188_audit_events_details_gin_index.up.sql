-- GIN index on audit_events.details enables efficient containment
-- queries (@>) for searching inside the JSONB details column.
-- Example: SELECT * FROM audit_events WHERE details @> '{"job_id": "job-xyz"}';
--
-- Uses jsonb_path_ops (smaller index, supports @> only — sufficient for
-- audit search use cases). CONCURRENTLY avoids locking the table.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_events_details_gin
    ON audit_events USING gin (details jsonb_path_ops);
