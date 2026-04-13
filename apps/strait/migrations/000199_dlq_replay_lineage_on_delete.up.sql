-- Partial index supporting lookups of lineage pointers on job_runs. We keep
-- it partial (WHERE replayed_run_id IS NOT NULL) because the vast majority
-- of job_runs are never replayed; a sparse index is far cheaper to maintain
-- and still covers the DLQ admin endpoints' lookup pattern.
--
-- Must be CREATE INDEX CONCURRENTLY: job_runs is a hot, partitioned table
-- under constant write load. golang-migrate's postgres driver wraps each
-- migration file in a transaction, and CONCURRENTLY cannot run inside a
-- transaction — therefore this file intentionally contains only the
-- CONCURRENTLY statement (no schema_version bump), matching the pattern
-- established in 000200_project_rate_limits_updated_at_index.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_replayed_run_id
    ON job_runs(replayed_run_id) WHERE replayed_run_id IS NOT NULL;
