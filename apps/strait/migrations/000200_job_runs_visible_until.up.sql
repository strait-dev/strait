-- Phase 7: soft-delete column on job_runs. The per-org reaper used to DELETE
-- rows, which creates dead tuples scattered across every partition even
-- though pg_partman would eventually drop the whole partition anyway.
-- Instead we mask rows with visible_until = NOW() and let pg_partman be the
-- authoritative physical reaper. The column is intentionally NOT indexed so
-- the UPDATE stays HOT-update-eligible, avoiding the index bloat problem
-- Phase 3 solved for status transitions.

ALTER TABLE job_runs
    ADD COLUMN IF NOT EXISTS visible_until TIMESTAMPTZ;

COMMENT ON COLUMN job_runs.visible_until IS
    'Soft-delete marker. NULL means visible. When set, the row is hidden from '
    'user-facing listings but still physically present until pg_partman drops '
    'the containing partition.';
