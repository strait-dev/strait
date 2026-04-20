-- Terminal run history table. Rows are archived from the hot job_runs table
-- once they reach a terminal state and pass retention, reducing dead-tuple
-- churn in the actively-polled partitions.

CREATE TABLE IF NOT EXISTS job_runs_history (
    id                        TEXT NOT NULL,
    job_id                    TEXT NOT NULL,
    project_id                TEXT NOT NULL,
    status                    TEXT NOT NULL,
    attempt                   INT NOT NULL DEFAULT 1,
    payload                   JSONB,
    result                    JSONB,
    metadata                  JSONB NOT NULL DEFAULT '{}',
    error                     TEXT,
    error_class               TEXT,
    triggered_by              TEXT NOT NULL DEFAULT 'manual',
    scheduled_at              TIMESTAMPTZ,
    started_at                TIMESTAMPTZ,
    finished_at               TIMESTAMPTZ,
    heartbeat_at              TIMESTAMPTZ,
    next_retry_at             TIMESTAMPTZ,
    expires_at                TIMESTAMPTZ,
    parent_run_id             TEXT,
    priority                  INT NOT NULL DEFAULT 0,
    idempotency_key           TEXT,
    job_version               INT NOT NULL DEFAULT 1,
    workflow_step_run_id      TEXT,
    execution_trace           JSONB,
    debug_mode                BOOLEAN NOT NULL DEFAULT false,
    continuation_of           TEXT,
    lineage_depth             INT NOT NULL DEFAULT 0,
    tags                      JSONB NOT NULL DEFAULT '{}'::jsonb,
    job_version_id            TEXT,
    created_by                TEXT,
    concurrency_key           TEXT,
    batch_id                  TEXT,
    execution_mode            TEXT,
    machine_id                TEXT,
    deployment_id             TEXT,
    pinned_image_uri          TEXT,
    pinned_image_digest       TEXT,
    is_rollback               BOOLEAN NOT NULL DEFAULT false,
    replayed_run_id           UUID,
    max_attempts_override     INT,
    timeout_secs_override     INT,
    retry_backoff             TEXT,
    retry_initial_delay_secs  INT,
    retry_max_delay_secs      INT,
    visible_until             TIMESTAMPTZ,
    job_enabled               BOOLEAN,
    job_paused                BOOLEAN,
    job_max_concurrency       INT,
    job_max_concurrency_per_key INT,
    created_at                TIMESTAMPTZ NOT NULL,
    archived_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id)
);

CREATE INDEX IF NOT EXISTS idx_job_runs_history_archived_at
    ON job_runs_history (archived_at);
CREATE INDEX IF NOT EXISTS idx_job_runs_history_project_created
    ON job_runs_history (project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_job_runs_history_job_created
    ON job_runs_history (job_id, created_at DESC);

ALTER TABLE job_runs_history SET (fillfactor = 100, autovacuum_vacuum_scale_factor = 0.1);

UPDATE schema_version SET version = 218, updated_at = NOW();
