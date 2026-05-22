-- safety-ok: reverse the shard chain support introduced in 000257. The
-- supporting partial index is dropped first, then the unique anchor
-- index is restored to its pre-shard shape (project, epoch, action).
DROP INDEX IF EXISTS idx_audit_events_shard_chain;

-- safety-ok: same swap pattern as the up migration; ACCESS EXCLUSIVE on
-- the anchor partial index for a brief window.
DROP INDEX IF EXISTS idx_audit_events_anchor_unique;

-- safety-ok: restore the original index shape from migration 000196.
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_events_anchor_unique
    ON audit_events (project_id, rotation_epoch, action)
    WHERE is_anchor = TRUE;

-- safety-ok: DROP COLUMN is metadata-only for a NOT NULL DEFAULT column;
-- no row rewrite needed.
ALTER TABLE audit_events
    DROP COLUMN IF EXISTS shard_id;

-- safety-ok: same DROP COLUMN treatment for the deadletter mirror.
ALTER TABLE audit_events_deadletter
    DROP COLUMN IF EXISTS shard_id;
