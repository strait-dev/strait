WITH ranked_global_job_secrets AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY project_id, environment, secret_key
            ORDER BY created_at DESC, id DESC
        ) AS rn
    FROM job_secrets
    WHERE job_id IS NULL
)
DELETE FROM job_secrets js
USING ranked_global_job_secrets ranked
WHERE js.id = ranked.id
  AND ranked.rn > 1;
