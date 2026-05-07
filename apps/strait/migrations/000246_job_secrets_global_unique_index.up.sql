CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_job_secrets_global_unique
    ON job_secrets (project_id, environment, secret_key)
    WHERE job_id IS NULL;
