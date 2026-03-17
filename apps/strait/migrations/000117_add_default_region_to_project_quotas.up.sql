-- Add default_region to project_quotas for per-project region configuration.
ALTER TABLE project_quotas ADD COLUMN IF NOT EXISTS default_region TEXT;
