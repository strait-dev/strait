-- audit_events_deadletter retention/reclaim hardening.
--
-- attempt_count tracks how many times the reclaimer has tried (and failed)
-- to replay this DLQ row into the primary audit_events chain. Combined with
-- a max-attempts cap in the reclaimer, this prevents a permanently-poisoned
-- row from being retried in every tick forever.
--
-- reclaimed_event_id stores the new audit_events.id produced by a successful
-- reclaim. The presence of a non-NULL value means a chain row already exists
-- for this DLQ entry — a subsequent replay must skip the chain insert and
-- only delete the DLQ row, preventing duplicate chain entries on retry.
ALTER TABLE audit_events_deadletter
    ADD COLUMN IF NOT EXISTS attempt_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS reclaimed_event_id TEXT NULL;

-- Index supports the retention reaper sweep filtering by (project_id, created_at).
-- The existing idx_audit_events_deadletter_project_queued is keyed on queued_at,
-- which is when the row landed in the DLQ; created_at is the original event
-- timestamp, which is what AUDIT_DLQ_MAX_AGE_DAYS checks against.
CREATE INDEX IF NOT EXISTS idx_audit_events_deadletter_project_created
    ON audit_events_deadletter(project_id, created_at);
