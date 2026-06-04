WITH latest AS (
    SELECT DISTINCT ON (run_id) id
    FROM job_run_heartbeats
    WHERE cleared = FALSE
    ORDER BY run_id, id DESC
)
DELETE FROM job_run_heartbeats h
WHERE h.id NOT IN (SELECT id FROM latest);

DELETE FROM job_run_heartbeats
WHERE cleared = TRUE;

DROP INDEX IF EXISTS idx_job_run_heartbeats_stale;
DROP INDEX IF EXISTS idx_job_run_heartbeats_latest;

ALTER TABLE job_run_heartbeats
    DROP CONSTRAINT IF EXISTS job_run_heartbeats_pkey;

ALTER TABLE job_run_heartbeats
    ADD PRIMARY KEY (run_id);

ALTER TABLE job_run_heartbeats
    DROP COLUMN IF EXISTS cleared;

ALTER TABLE job_run_heartbeats
    DROP COLUMN IF EXISTS id;
