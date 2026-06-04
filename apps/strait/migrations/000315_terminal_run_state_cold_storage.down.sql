DROP VIEW IF EXISTS job_run_read_state;

INSERT INTO job_run_state (
    run_id,
    project_id,
    job_id,
    status,
    attempt,
    priority,
    scheduled_at,
    started_at,
    finished_at,
    heartbeat_at,
    next_retry_at,
    expires_at,
    concurrency_key,
    execution_mode,
    queue_name,
    environment_id,
    job_enabled,
    job_paused,
    job_max_concurrency,
    job_max_concurrency_per_key,
    ready_generation,
    updated_at
)
SELECT
    run_id,
    project_id,
    job_id,
    status,
    attempt,
    priority,
    scheduled_at,
    started_at,
    finished_at,
    heartbeat_at,
    next_retry_at,
    expires_at,
    concurrency_key,
    execution_mode,
    queue_name,
    environment_id,
    job_enabled,
    job_paused,
    job_max_concurrency,
    job_max_concurrency_per_key,
    ready_generation,
    updated_at
FROM job_run_terminal_state
ON CONFLICT (run_id) DO NOTHING;

DROP TABLE IF EXISTS job_run_terminal_state;

CREATE OR REPLACE VIEW job_run_read_state AS
SELECT
    run_id,
    project_id,
    job_id,
    status,
    attempt,
    priority,
    scheduled_at,
    started_at,
    finished_at,
    heartbeat_at,
    next_retry_at,
    expires_at,
    concurrency_key,
    execution_mode,
    queue_name,
    environment_id,
    job_enabled,
    job_paused,
    job_max_concurrency,
    job_max_concurrency_per_key,
    lease_owner,
    lease_expires_at,
    ready_generation,
    updated_at
FROM job_run_state;
