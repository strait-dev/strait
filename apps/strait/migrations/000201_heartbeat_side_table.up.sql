-- Phase 8: unlogged heartbeat side table to eliminate job_runs churn on
-- the heartbeat hot path.
--
-- At 10k concurrent executing runs, every 5s heartbeat is ~2k UPDATEs/sec
-- on a partitioned, indexed job_runs table, generating dead tuples and
-- updating idx_runs_heartbeat constantly. Moving heartbeats to an UNLOGGED
-- table that is NOT crash-persistent gives us:
--   * No WAL writes for heartbeat traffic.
--   * Table truncated on crash -- identical semantics to a worker crash
--     which is exactly what the stale-reclaim loop is already designed for.
--   * PK-only row layout so upserts are tiny.

CREATE UNLOGGED TABLE IF NOT EXISTS job_run_heartbeats (
    run_id        TEXT PRIMARY KEY,
    heartbeat_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed from current job_runs state so workers don't immediately get
-- flagged as stale on first boot after the migration.
INSERT INTO job_run_heartbeats (run_id, heartbeat_at)
SELECT id, COALESCE(heartbeat_at, NOW())
FROM job_runs
WHERE status = 'executing'
ON CONFLICT (run_id) DO NOTHING;
