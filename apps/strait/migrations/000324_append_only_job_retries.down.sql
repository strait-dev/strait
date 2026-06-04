DROP FUNCTION IF EXISTS strait_run_retry_blocked(TEXT);

WITH latest AS (
    SELECT DISTINCT ON (run_id) id
    FROM job_retries
    WHERE cleared = FALSE
      AND next_retry_at IS NOT NULL
    ORDER BY run_id, id DESC
)
DELETE FROM job_retries r
WHERE r.id NOT IN (SELECT id FROM latest);

DELETE FROM job_retries
WHERE cleared = TRUE
   OR next_retry_at IS NULL;

DROP INDEX IF EXISTS idx_job_retries_due;
DROP INDEX IF EXISTS idx_job_retries_latest;

ALTER TABLE job_retries
    DROP CONSTRAINT IF EXISTS job_retries_pkey;

ALTER TABLE job_retries
    ADD PRIMARY KEY (run_id);

ALTER TABLE job_retries
    ALTER COLUMN next_retry_at SET NOT NULL;

ALTER TABLE job_retries
    DROP COLUMN IF EXISTS cleared;

ALTER TABLE job_retries
    DROP COLUMN IF EXISTS id;

CREATE INDEX IF NOT EXISTS idx_job_retries_next_retry_at
    ON job_retries (next_retry_at);
