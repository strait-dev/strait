ALTER TABLE jobs ADD COLUMN IF NOT EXISTS fallback_endpoint_url TEXT;

ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS fallback_endpoint_url TEXT;
