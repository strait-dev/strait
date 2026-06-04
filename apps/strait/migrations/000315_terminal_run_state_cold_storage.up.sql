CREATE TABLE IF NOT EXISTS job_run_terminal_state (
    run_id                      TEXT PRIMARY KEY,
    project_id                  TEXT NOT NULL,
    job_id                      TEXT NOT NULL,
    status                      TEXT NOT NULL,
    attempt                     INT NOT NULL DEFAULT 1,
    priority                    INT NOT NULL DEFAULT 0,
    scheduled_at                TIMESTAMPTZ,
    started_at                  TIMESTAMPTZ,
    finished_at                 TIMESTAMPTZ,
    heartbeat_at                TIMESTAMPTZ,
    next_retry_at               TIMESTAMPTZ,
    expires_at                  TIMESTAMPTZ,
    concurrency_key             TEXT NOT NULL DEFAULT '',
    execution_mode              TEXT NOT NULL DEFAULT 'http',
    queue_name                  TEXT NOT NULL DEFAULT 'default',
    environment_id              TEXT NOT NULL DEFAULT '',
    job_enabled                 BOOLEAN NOT NULL DEFAULT TRUE,
    job_paused                  BOOLEAN NOT NULL DEFAULT FALSE,
    job_max_concurrency         INT,
    job_max_concurrency_per_key INT,
    ready_generation            BIGINT NOT NULL DEFAULT 0,
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE job_run_terminal_state SET (
    fillfactor = 100,
    autovacuum_vacuum_threshold = 1000,
    autovacuum_vacuum_scale_factor = 0.05,
    autovacuum_analyze_threshold = 1000,
    autovacuum_analyze_scale_factor = 0.02
);

INSERT INTO job_run_terminal_state (
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
FROM job_run_state
WHERE status IN ('completed', 'failed', 'timed_out', 'crashed', 'system_failed', 'canceled', 'expired')
ON CONFLICT (run_id) DO NOTHING;

CREATE OR REPLACE VIEW job_run_read_state AS
SELECT
    s.run_id,
    s.project_id,
    s.job_id,
    CASE WHEN t.run_id IS NULL THEN s.status ELSE t.status END AS status,
    CASE WHEN t.run_id IS NULL THEN s.attempt ELSE t.attempt END AS attempt,
    CASE WHEN t.run_id IS NULL THEN s.priority ELSE t.priority END AS priority,
    CASE WHEN t.run_id IS NULL THEN s.scheduled_at ELSE t.scheduled_at END AS scheduled_at,
    CASE WHEN t.run_id IS NULL THEN s.started_at ELSE t.started_at END AS started_at,
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
    CASE WHEN t.run_id IS NULL THEN s.lease_owner ELSE NULL::TEXT END AS lease_owner,
    CASE WHEN t.run_id IS NULL THEN s.lease_expires_at ELSE NULL::TIMESTAMPTZ END AS lease_expires_at,
    CASE WHEN t.run_id IS NULL THEN s.ready_generation ELSE t.ready_generation END AS ready_generation,
    CASE WHEN t.run_id IS NULL THEN s.updated_at ELSE t.updated_at END AS updated_at
FROM job_run_state s
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
