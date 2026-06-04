CREATE TABLE IF NOT EXISTS job_run_state (
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
    job_enabled                 BOOLEAN NOT NULL DEFAULT TRUE,
    job_paused                  BOOLEAN NOT NULL DEFAULT FALSE,
    job_max_concurrency         INT,
    job_max_concurrency_per_key INT,
    lease_owner                 TEXT,
    lease_expires_at            TIMESTAMPTZ,
    ready_generation            BIGINT NOT NULL DEFAULT 0,
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_job_run_state_claim_http
    ON job_run_state(priority DESC, updated_at ASC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'http';

CREATE INDEX IF NOT EXISTS idx_job_run_state_project_claim
    ON job_run_state(project_id, priority DESC, updated_at ASC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'http';

CREATE INDEX IF NOT EXISTS idx_job_run_state_worker_claim
    ON job_run_state(project_id, queue_name, priority DESC, updated_at ASC, run_id ASC)
    WHERE status = 'queued' AND execution_mode = 'worker';

CREATE INDEX IF NOT EXISTS idx_job_run_state_lease_expiry
    ON job_run_state(lease_expires_at)
    WHERE lease_expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS job_run_lifecycle_events (
    id          BIGSERIAL PRIMARY KEY,
    run_id      TEXT NOT NULL,
    from_status TEXT,
    to_status   TEXT NOT NULL,
    attempt     INT NOT NULL,
    fields      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_job_run_lifecycle_events_run_created
    ON job_run_lifecycle_events(run_id, created_at DESC, id DESC);

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
    job_enabled,
    job_paused,
    job_max_concurrency,
    job_max_concurrency_per_key
)
SELECT
    jr.id,
    jr.project_id,
    jr.job_id,
    jr.status,
    jr.attempt,
    jr.priority,
    jr.scheduled_at,
    jr.started_at,
    jr.finished_at,
    jr.heartbeat_at,
    jr.next_retry_at,
    jr.expires_at,
    COALESCE(jr.concurrency_key, ''),
    COALESCE(NULLIF(jr.execution_mode, ''), 'http'),
    COALESCE(NULLIF(jr.queue_name, ''), 'default'),
    COALESCE(jr.job_enabled, TRUE),
    COALESCE(jr.job_paused, FALSE),
    jr.job_max_concurrency,
    jr.job_max_concurrency_per_key
FROM job_runs jr
ON CONFLICT (run_id) DO NOTHING;

CREATE OR REPLACE FUNCTION sync_job_run_state_from_job_runs()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        DELETE FROM job_run_state WHERE run_id = OLD.id;
        RETURN OLD;
    END IF;

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
        job_enabled,
        job_paused,
        job_max_concurrency,
        job_max_concurrency_per_key,
        updated_at
    )
    VALUES (
        NEW.id,
        NEW.project_id,
        NEW.job_id,
        NEW.status,
        NEW.attempt,
        NEW.priority,
        NEW.scheduled_at,
        NEW.started_at,
        NEW.finished_at,
        NEW.heartbeat_at,
        NEW.next_retry_at,
        NEW.expires_at,
        COALESCE(NEW.concurrency_key, ''),
        COALESCE(NULLIF(NEW.execution_mode, ''), 'http'),
        COALESCE(NULLIF(NEW.queue_name, ''), 'default'),
        COALESCE(NEW.job_enabled, TRUE),
        COALESCE(NEW.job_paused, FALSE),
        NEW.job_max_concurrency,
        NEW.job_max_concurrency_per_key,
        NOW()
    )
    ON CONFLICT (run_id) DO UPDATE
    SET project_id = EXCLUDED.project_id,
        job_id = EXCLUDED.job_id,
        status = EXCLUDED.status,
        attempt = EXCLUDED.attempt,
        priority = EXCLUDED.priority,
        scheduled_at = EXCLUDED.scheduled_at,
        started_at = EXCLUDED.started_at,
        finished_at = EXCLUDED.finished_at,
        heartbeat_at = EXCLUDED.heartbeat_at,
        next_retry_at = EXCLUDED.next_retry_at,
        expires_at = EXCLUDED.expires_at,
        concurrency_key = EXCLUDED.concurrency_key,
        execution_mode = EXCLUDED.execution_mode,
        queue_name = EXCLUDED.queue_name,
        job_enabled = EXCLUDED.job_enabled,
        job_paused = EXCLUDED.job_paused,
        job_max_concurrency = EXCLUDED.job_max_concurrency,
        job_max_concurrency_per_key = EXCLUDED.job_max_concurrency_per_key,
        updated_at = NOW();

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_job_runs_state_sync_insert ON job_runs;
CREATE TRIGGER trg_job_runs_state_sync_insert
AFTER INSERT ON job_runs
FOR EACH ROW
EXECUTE FUNCTION sync_job_run_state_from_job_runs();

DROP TRIGGER IF EXISTS trg_job_runs_state_sync_update ON job_runs;
CREATE TRIGGER trg_job_runs_state_sync_update
AFTER UPDATE OF status, attempt, priority, scheduled_at, started_at, finished_at, heartbeat_at, next_retry_at, expires_at, concurrency_key, execution_mode, queue_name, job_enabled, job_paused, job_max_concurrency, job_max_concurrency_per_key ON job_runs
FOR EACH ROW
EXECUTE FUNCTION sync_job_run_state_from_job_runs();

DROP TRIGGER IF EXISTS trg_job_runs_state_sync_delete ON job_runs;
CREATE TRIGGER trg_job_runs_state_sync_delete
AFTER DELETE ON job_runs
FOR EACH ROW
EXECUTE FUNCTION sync_job_run_state_from_job_runs();
