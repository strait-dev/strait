DROP FUNCTION IF EXISTS strait_next_run_cache_version(TEXT);

WITH latest AS (
    SELECT DISTINCT ON (run_id) id
    FROM job_run_cache_versions
    ORDER BY run_id, id DESC
)
DELETE FROM job_run_cache_versions v
WHERE v.id NOT IN (SELECT id FROM latest);

DROP INDEX IF EXISTS idx_job_run_cache_versions_latest;

ALTER TABLE job_run_cache_versions
    DROP CONSTRAINT IF EXISTS job_run_cache_versions_pkey;

ALTER TABLE job_run_cache_versions
    ADD PRIMARY KEY (run_id);

ALTER TABLE job_run_cache_versions
    DROP COLUMN IF EXISTS id;
