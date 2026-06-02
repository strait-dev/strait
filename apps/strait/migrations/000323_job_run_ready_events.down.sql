DROP VIEW IF EXISTS job_run_read_state;

DROP TABLE IF EXISTS job_run_ready_events;

CREATE OR REPLACE VIEW job_run_read_state AS
SELECT
    s.run_id,
    s.project_id,
    s.job_id,
    CASE
        WHEN t.run_id IS NOT NULL THEN t.status
        WHEN c.run_id IS NOT NULL AND s.status = 'queued' THEN 'executing'
        ELSE s.status
    END AS status,
    CASE WHEN t.run_id IS NULL THEN COALESCE(c.attempt, s.attempt) ELSE t.attempt END AS attempt,
    CASE WHEN t.run_id IS NULL THEN s.priority ELSE t.priority END AS priority,
    CASE WHEN t.run_id IS NULL THEN s.scheduled_at ELSE t.scheduled_at END AS scheduled_at,
    CASE WHEN t.run_id IS NULL THEN COALESCE(c.started_at, s.started_at) ELSE t.started_at END AS started_at,
    CASE WHEN t.run_id IS NULL THEN s.finished_at ELSE t.finished_at END AS finished_at,
    CASE WHEN t.run_id IS NULL THEN s.heartbeat_at ELSE t.heartbeat_at END AS heartbeat_at,
    CASE WHEN t.run_id IS NULL THEN s.next_retry_at ELSE t.next_retry_at END AS next_retry_at,
    CASE WHEN t.run_id IS NULL THEN s.expires_at ELSE t.expires_at END AS expires_at,
    CASE WHEN t.run_id IS NULL THEN s.concurrency_key ELSE t.concurrency_key END AS concurrency_key,
    CASE WHEN t.run_id IS NULL THEN s.execution_mode ELSE t.execution_mode END AS execution_mode,
    CASE WHEN t.run_id IS NULL THEN s.queue_name ELSE t.queue_name END AS queue_name,
    CASE WHEN t.run_id IS NULL THEN s.environment_id ELSE t.environment_id END AS environment_id,
    CASE WHEN t.run_id IS NULL THEN s.job_enabled ELSE t.job_enabled END AS job_enabled,
    CASE WHEN t.run_id IS NULL THEN s.job_paused ELSE t.job_paused END AS job_paused,
    CASE WHEN t.run_id IS NULL THEN s.job_max_concurrency ELSE t.job_max_concurrency END AS job_max_concurrency,
    CASE WHEN t.run_id IS NULL THEN s.job_max_concurrency_per_key ELSE t.job_max_concurrency_per_key END AS job_max_concurrency_per_key,
    NULL::TEXT AS lease_owner,
    NULL::TIMESTAMPTZ AS lease_expires_at,
    CASE WHEN t.run_id IS NULL THEN s.ready_generation ELSE t.ready_generation END AS ready_generation,
    CASE WHEN t.run_id IS NULL THEN GREATEST(s.updated_at, COALESCE(c.started_at, s.updated_at)) ELSE t.updated_at END AS updated_at
FROM job_run_state s
LEFT JOIN job_run_active_claims c
    ON c.run_id = s.run_id
   AND c.ready_generation = s.ready_generation
LEFT JOIN job_run_terminal_state t ON t.run_id = s.run_id
UNION ALL
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
    NULL::TEXT AS lease_owner,
    NULL::TIMESTAMPTZ AS lease_expires_at,
    ready_generation,
    updated_at
FROM job_run_terminal_state t
WHERE NOT EXISTS (
    SELECT 1 FROM job_run_state s WHERE s.run_id = t.run_id
);
