-- Add workers and worker_tasks tables for orchestration-only mode.
-- Workers register themselves and are assigned tasks (job runs) to execute.

CREATE TABLE IF NOT EXISTS workers (
    id              TEXT        PRIMARY KEY,
    project_id      TEXT        NOT NULL,
    queue_name      TEXT        NOT NULL DEFAULT 'default',
    hostname        TEXT        NOT NULL DEFAULT '',
    version         TEXT        NOT NULL DEFAULT '',
    status          TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'draining', 'offline')),
    last_seen_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workers_project_queue ON workers (project_id, queue_name, status);
CREATE INDEX IF NOT EXISTS idx_workers_last_seen ON workers (last_seen_at);

CREATE TABLE IF NOT EXISTS worker_tasks (
    id              TEXT        PRIMARY KEY,
    worker_id       TEXT        NOT NULL REFERENCES workers(id) ON DELETE CASCADE,
    run_id          TEXT        NOT NULL,
    project_id      TEXT        NOT NULL,
    status          TEXT        NOT NULL DEFAULT 'assigned' CHECK (status IN ('assigned', 'accepted', 'completed', 'failed')),
    assigned_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    accepted_at     TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_worker_tasks_worker ON worker_tasks (worker_id, status);
CREATE INDEX IF NOT EXISTS idx_worker_tasks_run ON worker_tasks (run_id);
