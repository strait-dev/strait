ALTER TABLE job_versions DROP COLUMN IF EXISTS fallback_endpoint_url;

ALTER TABLE jobs DROP COLUMN IF EXISTS fallback_endpoint_url;
