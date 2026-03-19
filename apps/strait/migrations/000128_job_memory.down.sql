ALTER TABLE project_quotas DROP COLUMN IF EXISTS max_memory_per_key_bytes;
ALTER TABLE project_quotas DROP COLUMN IF EXISTS max_memory_per_job_bytes;

DROP TABLE IF EXISTS job_memory;
