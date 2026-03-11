-- Revert partitioned job_runs back to a regular table with self-referencing FK.

-- Step 1: Rename partitioned table.
ALTER TABLE job_runs RENAME TO job_runs_partitioned;

-- Step 2: Recreate the original non-partitioned table.
CREATE TABLE job_runs (
    id             TEXT        PRIMARY KEY,
    job_id         TEXT        NOT NULL REFERENCES jobs(id),
    project_id     TEXT        NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'queued',
    attempt        INT         NOT NULL DEFAULT 1,
    payload        JSONB,
    result         JSONB,
    error          TEXT,
    triggered_by   TEXT        NOT NULL DEFAULT 'manual',
    scheduled_at   TIMESTAMPTZ,
    started_at     TIMESTAMPTZ,
    finished_at    TIMESTAMPTZ,
    heartbeat_at   TIMESTAMPTZ,
    next_retry_at  TIMESTAMPTZ,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    parent_run_id  TEXT,
    job_version    INT         NOT NULL DEFAULT 1,
    job_version_id TEXT        NOT NULL DEFAULT '',
    max_attempts   INT         NOT NULL DEFAULT 3,
    priority       INT         NOT NULL DEFAULT 0,
    error_class    TEXT,
    execution_trace JSONB,
    idempotency_key TEXT,
    workflow_step_run_id TEXT
);

-- Step 3: Copy data back.
INSERT INTO job_runs SELECT * FROM job_runs_partitioned;

-- Step 4: Drop partitioned table and its partitions.
DROP TABLE job_runs_partitioned CASCADE;

-- Step 5: Restore indexes.
CREATE INDEX idx_runs_queue ON job_runs(created_at ASC) WHERE status = 'queued';
CREATE INDEX idx_runs_project_status ON job_runs(project_id, status, created_at DESC);
CREATE INDEX idx_runs_heartbeat ON job_runs(heartbeat_at) WHERE status = 'executing';
CREATE INDEX idx_runs_expires ON job_runs(expires_at) WHERE expires_at IS NOT NULL AND status IN ('delayed', 'queued');
CREATE INDEX idx_job_runs_parent_run_id ON job_runs(parent_run_id) WHERE parent_run_id IS NOT NULL;

-- Step 6: Restore self-referencing FK.
ALTER TABLE job_runs ADD CONSTRAINT job_runs_parent_run_id_fkey
    FOREIGN KEY (parent_run_id) REFERENCES job_runs(id);

-- Step 7: Remove pg_partman config (ignore if not present).
DELETE FROM partman.part_config WHERE parent_table = 'public.job_runs';
