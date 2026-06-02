ALTER TABLE job_run_heartbeats
    ADD COLUMN IF NOT EXISTS id BIGSERIAL;

ALTER TABLE job_run_heartbeats
    ADD COLUMN IF NOT EXISTS cleared BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE job_run_heartbeats
    DROP CONSTRAINT IF EXISTS job_run_heartbeats_pkey;

ALTER TABLE job_run_heartbeats
    ADD PRIMARY KEY (id);

CREATE INDEX IF NOT EXISTS idx_job_run_heartbeats_latest
    ON job_run_heartbeats(run_id, id DESC);

CREATE INDEX IF NOT EXISTS idx_job_run_heartbeats_stale
    ON job_run_heartbeats(heartbeat_at, id)
    WHERE cleared = FALSE;
