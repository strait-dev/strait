-- Broaden the anchor uniqueness guard to allow distinct anchor actions to
-- coexist within the same epoch.
--
-- Migration 000191 created idx_audit_events_anchor_unique on
-- (project_id, rotation_epoch) WHERE is_anchor = TRUE to prevent two
-- concurrent rotations from racing past the max(rotation_epoch) read and
-- both inserting the same epoch's anchor row.
--
-- That goal still stands, but the original index also rejected legitimate
-- multi-action anchors. A retention trim emits a tombstone anchor with
-- action='audit.retention_trimmed' inside the *current* epoch (no epoch
-- bump). When a retention reaper runs in the same epoch as a rotation,
-- both anchors collide on the partial index even though they record
-- semantically distinct events.
--
-- Including action in the index restores the per-action uniqueness:
-- - at most one audit.key_rotated anchor per (project, epoch) — still
--   prevents the rotation race the original index was designed for, since
--   only RotateAuditSigningKey writes that action.
-- - at most one audit.retention_trimmed anchor per (project, epoch) — a
--   second retention trim in the same epoch is a logic bug worth
--   surfacing, so the partial index continues to fail-loud.
-- - any future anchor action gets the same per-(project, epoch) guard
--   automatically.
--
-- CONCURRENTLY is intentionally omitted: golang-migrate wraps each
-- migration in a transaction, and CONCURRENTLY cannot run there. The
-- partial index is small (only anchor rows are indexed; anchors are rare
-- per project), so the brief ACCESS EXCLUSIVE lock is acceptable.

DROP INDEX IF EXISTS idx_audit_events_anchor_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_events_anchor_unique
    ON audit_events (project_id, rotation_epoch, action)
    WHERE is_anchor = TRUE;
