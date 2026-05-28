DROP TRIGGER IF EXISTS cache_version_bump ON workflow_step_runs;
DROP TRIGGER IF EXISTS cache_version_bump ON workflow_runs;
DROP TRIGGER IF EXISTS cache_version_bump ON job_runs;
DROP TRIGGER IF EXISTS cache_version_bump ON job_dependencies;
DROP TRIGGER IF EXISTS cache_version_bump ON jobs;
DROP TRIGGER IF EXISTS cache_version_bump ON organization_subscriptions;
DROP TRIGGER IF EXISTS cache_version_bump ON project_quotas;
DROP TRIGGER IF EXISTS cache_version_bump ON tag_policies;
DROP TRIGGER IF EXISTS cache_version_bump ON resource_policies;
DROP TRIGGER IF EXISTS cache_version_bump ON project_member_roles;
DROP TRIGGER IF EXISTS cache_version_bump ON project_roles;
DROP TRIGGER IF EXISTS cache_version_bump ON api_keys;

DROP FUNCTION IF EXISTS bump_cache_version();

ALTER TABLE workflow_step_runs DROP COLUMN IF EXISTS cache_version;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS cache_version;
ALTER TABLE job_runs_history DROP COLUMN IF EXISTS cache_version;
ALTER TABLE job_runs DROP COLUMN IF EXISTS cache_version;
ALTER TABLE job_dependencies DROP COLUMN IF EXISTS cache_version;
ALTER TABLE jobs DROP COLUMN IF EXISTS cache_version;
ALTER TABLE organization_subscriptions DROP COLUMN IF EXISTS cache_version;
ALTER TABLE project_quotas DROP COLUMN IF EXISTS cache_version;
ALTER TABLE tag_policies DROP COLUMN IF EXISTS cache_version;
ALTER TABLE resource_policies DROP COLUMN IF EXISTS cache_version;
ALTER TABLE project_member_roles DROP COLUMN IF EXISTS cache_version;
ALTER TABLE project_roles DROP COLUMN IF EXISTS cache_version;
ALTER TABLE api_keys DROP COLUMN IF EXISTS cache_version;
