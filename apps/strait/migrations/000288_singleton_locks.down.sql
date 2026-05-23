DROP INDEX IF EXISTS idx_job_runs_singleton_waiters;

ALTER TABLE workflow_runs DROP COLUMN IF EXISTS singleton_key;

ALTER TABLE job_runs DROP COLUMN IF EXISTS singleton_key;

ALTER TABLE workflow_versions
    DROP COLUMN IF EXISTS singleton_max_queue_depth,
    DROP COLUMN IF EXISTS singleton_on_conflict,
    DROP COLUMN IF EXISTS singleton_key_expr;

ALTER TABLE workflows DROP CONSTRAINT IF EXISTS workflows_singleton_on_conflict_check;

ALTER TABLE workflows
    DROP COLUMN IF EXISTS singleton_max_queue_depth,
    DROP COLUMN IF EXISTS singleton_on_conflict,
    DROP COLUMN IF EXISTS singleton_key_expr;

ALTER TABLE job_versions
    DROP COLUMN IF EXISTS singleton_max_queue_depth,
    DROP COLUMN IF EXISTS singleton_on_conflict,
    DROP COLUMN IF EXISTS singleton_key_expr;

ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_singleton_on_conflict_check;

ALTER TABLE jobs
    DROP COLUMN IF EXISTS singleton_max_queue_depth,
    DROP COLUMN IF EXISTS singleton_on_conflict,
    DROP COLUMN IF EXISTS singleton_key_expr;

DROP TABLE IF EXISTS singleton_locks;
