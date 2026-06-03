ALTER TABLE job_run_heartbeats
    ADD COLUMN IF NOT EXISTS id BIGSERIAL;

-- safety-ok: job_run_heartbeats is an unlogged side table migrated during startup before heartbeat GC uses append-only rows.
ALTER TABLE job_run_heartbeats
    ADD COLUMN IF NOT EXISTS cleared BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE job_run_heartbeats
    DROP CONSTRAINT IF EXISTS job_run_heartbeats_pkey;

ALTER TABLE job_run_heartbeats
    ADD PRIMARY KEY (id);

-- safety-ok: heartbeat append-only indexes are built during the startup migration that switches heartbeat storage shape.
CREATE INDEX IF NOT EXISTS idx_job_run_heartbeats_latest
    ON job_run_heartbeats(run_id, id DESC);

-- safety-ok: heartbeat append-only indexes are built during the startup migration that switches heartbeat storage shape.
CREATE INDEX IF NOT EXISTS idx_job_run_heartbeats_stale
    ON job_run_heartbeats(heartbeat_at, id)
    WHERE cleared = FALSE;
