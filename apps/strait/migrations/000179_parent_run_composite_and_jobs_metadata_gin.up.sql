-- ListChildRuns in internal/store/runs.go filters on parent_run_id and
-- orders by created_at ASC (with an optional created_at cursor). The
-- existing idx_job_runs_parent_run_id (000066) is single-column and
-- supports the equality filter but cannot satisfy the ordering without a
-- sort step. This composite partial index covers both.
CREATE INDEX IF NOT EXISTS idx_job_runs_parent_run_id_created_at
    ON job_runs (parent_run_id, created_at ASC)
    WHERE parent_run_id IS NOT NULL;

-- The single-column partial index is a strict prefix of the composite and
-- becomes redundant once the composite is in place.
DROP INDEX IF EXISTS idx_job_runs_parent_run_id;

-- jobs.default_run_metadata was added in migration 000083 as a JSONB
-- column but never received a GIN index. job_runs.metadata has GIN with
-- jsonb_path_ops from migration 000091, but the jobs column was missed.
-- Any containment query against default_run_metadata currently sequential
-- scans the jobs table.
CREATE INDEX IF NOT EXISTS idx_jobs_default_run_metadata_gin
    ON jobs USING GIN (default_run_metadata jsonb_path_ops);
