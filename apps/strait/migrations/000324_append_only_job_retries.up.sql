ALTER TABLE job_retries
    ADD COLUMN IF NOT EXISTS id BIGSERIAL;

-- safety-ok: job_retries is migrated during startup before the PgQue retry side-table path serves traffic.
ALTER TABLE job_retries
    ADD COLUMN IF NOT EXISTS cleared BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE job_retries
    ALTER COLUMN next_retry_at DROP NOT NULL;

ALTER TABLE job_retries
    DROP CONSTRAINT IF EXISTS job_retries_pkey;

ALTER TABLE job_retries
    ADD PRIMARY KEY (id);

DROP INDEX IF EXISTS idx_job_retries_next_retry_at;

-- safety-ok: job_retries append-only indexes are built during the startup migration that switches retry storage shape.
CREATE INDEX IF NOT EXISTS idx_job_retries_latest
    ON job_retries(run_id, id DESC);

-- safety-ok: job_retries append-only indexes are built during the startup migration that switches retry storage shape.
CREATE INDEX IF NOT EXISTS idx_job_retries_due
    ON job_retries(next_retry_at, id)
    WHERE cleared = FALSE AND next_retry_at IS NOT NULL;

CREATE OR REPLACE FUNCTION strait_run_retry_blocked(p_run_id TEXT)
RETURNS BOOLEAN
LANGUAGE sql
STABLE
AS $$
    SELECT EXISTS (
        SELECT 1
        FROM job_retries r
        WHERE r.run_id = p_run_id
          AND r.cleared = FALSE
          AND r.next_retry_at > NOW()
          AND NOT EXISTS (
              SELECT 1
              FROM job_retries newer
              WHERE newer.run_id = r.run_id
                AND newer.id > r.id
          )
    );
$$;
