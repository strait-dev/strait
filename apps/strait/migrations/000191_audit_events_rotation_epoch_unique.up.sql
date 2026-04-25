-- Enforce at most one anchor row per (project_id, rotation_epoch).
--
-- RotateAuditSigningKey computes the next epoch as max(rotation_epoch)+1
-- and then inserts an is_anchor=TRUE row under that epoch. Without a
-- uniqueness guarantee, two concurrent rotations that interleave between
-- the max() read and the INSERT can produce two anchor rows under the
-- same new epoch — a chain-integrity hole that a forger could also
-- exploit to mask a tampered rotation boundary.
--
-- This partial unique index permits unlimited non-anchor rows per
-- (project, epoch) (the normal case — every post-rotation event lives in
-- the same epoch as the anchor), while rejecting a second anchor under
-- the same epoch with Postgres 23505 so the loser can retry under the
-- serializing advisory lock.
--
-- CONCURRENTLY avoids the ACCESS EXCLUSIVE / SHARE lock that would
-- otherwise block INSERT traffic on audit_events during index build.
-- audit_events is a hot write path — a blocking index build can stall
-- the whole emit pipeline on a large table. golang-migrate's postgres
-- driver dispatches each migration via ExecContext (no transactional
-- wrapper), so CONCURRENTLY is accepted here. The IF NOT EXISTS makes
-- the migration idempotent on retry.

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_events_anchor_unique
    ON audit_events (project_id, rotation_epoch)
    WHERE is_anchor = TRUE;
