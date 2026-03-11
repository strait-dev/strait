-- Revert partitioned job_runs back to a regular table with self-referencing FK.

-- Step 1: Rename partitioned table.
ALTER TABLE job_runs RENAME TO job_runs_partitioned;

-- Step 2: Recreate the original non-partitioned table.
CREATE TABLE job_runs (
    id                      TEXT        PRIMARY KEY,
    job_id                  TEXT        NOT NULL REFERENCES jobs(id),
    project_id              TEXT        NOT NULL,
    status                  TEXT        NOT NULL DEFAULT 'queued',
    attempt                 INT         NOT NULL DEFAULT 1,
    payload                 JSONB,
    result                  JSONB,
    error                   TEXT,
    triggered_by            TEXT        NOT NULL DEFAULT 'manual',
    scheduled_at            TIMESTAMPTZ,
    started_at              TIMESTAMPTZ,
    finished_at             TIMESTAMPTZ,
    heartbeat_at            TIMESTAMPTZ,
    next_retry_at           TIMESTAMPTZ,
    expires_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    parent_run_id           TEXT,
    priority                INT         NOT NULL DEFAULT 0,
    idempotency_key         TEXT,
    job_version             INT         NOT NULL DEFAULT 1,
    workflow_step_run_id    TEXT,
    error_class             TEXT,
    metadata                JSONB       NOT NULL DEFAULT '{}',
    execution_trace         JSONB,
    debug_mode              BOOLEAN     NOT NULL DEFAULT false,
    continuation_of         TEXT,
    lineage_depth           INT         NOT NULL DEFAULT 0,
    max_attempts_override   INT,
    timeout_secs_override   INT,
    retry_backoff           TEXT,
    retry_initial_delay_secs INT,
    retry_max_delay_secs    INT,
    created_by              TEXT,
    job_version_id          TEXT,
    tags                    JSONB       NOT NULL DEFAULT '{}'::jsonb
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

-- Step 7: Remove pg_partman config if present.
DO $$
BEGIN
  DELETE FROM partman.part_config WHERE parent_table = 'public.job_runs';
EXCEPTION WHEN OTHERS THEN
  RAISE NOTICE 'partman cleanup skipped: %', SQLERRM;
END;
$$;
