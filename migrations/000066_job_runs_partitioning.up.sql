-- Prepare job_runs for range partitioning by created_at.
-- Partitioning requires: (1) PK includes partition key, (2) no self-referencing FKs.
-- Parent-run integrity is enforced at the application layer after this migration.

-- Step 1: Drop self-referencing FK on parent_run_id.
-- This FK prevents partitioning because cross-partition FK references are not
-- supported in PostgreSQL. The application layer will enforce parent existence.
ALTER TABLE job_runs DROP CONSTRAINT IF EXISTS job_runs_parent_run_id_fkey;

-- Step 2: Enable pg_partman for automated partition management.
-- pg_partman handles creation/retention of monthly partitions.
CREATE EXTENSION IF NOT EXISTS pg_partman;

-- Step 3: Create the partitioned table structure.
-- We rename the existing table, create a new partitioned table with the same
-- schema, migrate data, then drop the old table.
ALTER TABLE job_runs RENAME TO job_runs_legacy;

CREATE TABLE job_runs (
    id             TEXT        NOT NULL,
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
    workflow_step_run_id TEXT,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Step 4: Create initial monthly partitions.
-- Default partition catches any rows outside defined ranges.
CREATE TABLE job_runs_default PARTITION OF job_runs DEFAULT;

-- Create partitions for current and next 3 months.
DO $$
DECLARE
    start_date DATE;
    end_date DATE;
    partition_name TEXT;
BEGIN
    FOR i IN 0..3 LOOP
        start_date := DATE_TRUNC('month', CURRENT_DATE) + (i || ' months')::INTERVAL;
        end_date := start_date + '1 month'::INTERVAL;
        partition_name := 'job_runs_p' || TO_CHAR(start_date, 'YYYY_MM');

        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF job_runs FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
    END LOOP;
END $$;

-- Step 5: Copy data from legacy table.
INSERT INTO job_runs SELECT * FROM job_runs_legacy;

-- Step 6: Recreate indexes on the partitioned table.
CREATE INDEX idx_runs_queue ON job_runs(created_at ASC) WHERE status = 'queued';
CREATE INDEX idx_runs_project_status ON job_runs(project_id, status, created_at DESC);
CREATE INDEX idx_runs_heartbeat ON job_runs(heartbeat_at) WHERE status = 'executing';
CREATE INDEX idx_runs_expires ON job_runs(expires_at) WHERE expires_at IS NOT NULL AND status IN ('delayed', 'queued');
CREATE INDEX idx_job_runs_parent_run_id ON job_runs(parent_run_id) WHERE parent_run_id IS NOT NULL;

-- Step 7: Configure pg_partman for automatic partition management.
-- Creates new monthly partitions 4 months ahead, retains indefinitely.
SELECT partman.create_parent(
    p_parent_table := 'public.job_runs',
    p_control := 'created_at',
    p_interval := '1 month',
    p_premake := 4
);

-- Step 8: Drop legacy table.
DROP TABLE job_runs_legacy;
