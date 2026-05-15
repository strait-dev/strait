-- Per-(project, resource_type) audit chain shards.
--
-- The audit chain currently serializes every emit in a project through a
-- single advisory lock (AdvisoryLockNsAuditChain). Hot projects emitting
-- audit events across many resource types contend on that lock even though
-- the chain integrity invariant only requires ordering within a chain, not
-- across unrelated chains.
--
-- shard_id partitions the chain. New events (schema v4+) set shard_id to
-- the row's resource_type and chain only against the tail of that shard.
-- Existing rows keep the default '' shard_id and continue to verify under
-- the legacy per-project path (frozen chain); the canonical HMAC form
-- branches on shard_id != '' to bind the shard identifier into the
-- signature, so a row's shard cannot be flipped without breaking its HMAC.
--
-- The expanded anchor uniqueness index includes shard_id so per-shard
-- anchors emitted by rotation and retention do not collide on a single
-- (project, epoch, action) slot. Within the legacy shard ('') the
-- uniqueness guarantee is unchanged.

-- safety-ok: ALTER TABLE ADD COLUMN with a constant DEFAULT is metadata-only
-- in PostgreSQL 11+. The new column is NOT NULL with default ''; existing
-- rows do not require a rewrite, and writers continue to populate '' until
-- the shard-aware CreateAuditEvent path lands in the next phase.
ALTER TABLE audit_events
    ADD COLUMN IF NOT EXISTS shard_id TEXT NOT NULL DEFAULT '';

-- safety-ok: same metadata-only ADD COLUMN treatment for the deadletter
-- mirror so reclaim paths can preserve the shard on requeue.
ALTER TABLE audit_events_deadletter
    ADD COLUMN IF NOT EXISTS shard_id TEXT NOT NULL DEFAULT '';

-- safety-ok: the existing partial index covers anchor rows only (a small
-- subset of audit_events). Dropping and recreating it takes a brief
-- ACCESS EXCLUSIVE lock per partition equivalent to the swap pattern used
-- in 000196. CONCURRENTLY is not viable: golang-migrate wraps each
-- migration in a transaction and CREATE/DROP INDEX CONCURRENTLY cannot
-- run inside a transaction block.
DROP INDEX IF EXISTS idx_audit_events_anchor_unique;

-- safety-ok: see DROP comment above. The replacement index extends the
-- uniqueness key with shard_id so per-shard rotation and retention
-- anchors can coexist within the same epoch without colliding. Legacy
-- rows (shard_id = '') retain their original per-(project, epoch,
-- action) uniqueness guarantee within the empty shard.
CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_events_anchor_unique
    ON audit_events (project_id, shard_id, rotation_epoch, action)
    WHERE is_anchor = TRUE;

-- safety-ok: supporting index for shard-scoped tail reads in
-- CreateAuditEvent's prev-hash lookup. Partial on shard_id != '' keeps
-- the index size proportional to sharded traffic only; legacy rows are
-- served by the existing (project_id, created_at) integrity index.
CREATE INDEX IF NOT EXISTS idx_audit_events_shard_chain
    ON audit_events (project_id, shard_id, rotation_epoch DESC, created_at DESC, id DESC)
    WHERE shard_id != '';
