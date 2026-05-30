ALTER TABLE workflow_step_runs ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE workflow_runs ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE job_runs_history ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE job_runs ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE job_dependencies ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE jobs ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE organization_subscriptions ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE project_quotas ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE tag_policies ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE resource_policies ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE project_member_roles ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE project_roles ALTER COLUMN cache_version DROP NOT NULL;
ALTER TABLE api_keys ALTER COLUMN cache_version DROP NOT NULL;

ALTER TABLE workflow_step_runs DROP CONSTRAINT IF EXISTS workflow_step_runs_cache_version_nn;
ALTER TABLE workflow_runs DROP CONSTRAINT IF EXISTS workflow_runs_cache_version_nn;
ALTER TABLE job_runs_history DROP CONSTRAINT IF EXISTS job_runs_history_cache_version_nn;
ALTER TABLE job_runs DROP CONSTRAINT IF EXISTS job_runs_cache_version_nn;
ALTER TABLE job_dependencies DROP CONSTRAINT IF EXISTS job_dependencies_cache_version_nn;
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_cache_version_nn;
ALTER TABLE organization_subscriptions DROP CONSTRAINT IF EXISTS organization_subscriptions_cache_version_nn;
ALTER TABLE project_quotas DROP CONSTRAINT IF EXISTS project_quotas_cache_version_nn;
ALTER TABLE tag_policies DROP CONSTRAINT IF EXISTS tag_policies_cache_version_nn;
ALTER TABLE resource_policies DROP CONSTRAINT IF EXISTS resource_policies_cache_version_nn;
ALTER TABLE project_member_roles DROP CONSTRAINT IF EXISTS project_member_roles_cache_version_nn;
ALTER TABLE project_roles DROP CONSTRAINT IF EXISTS project_roles_cache_version_nn;
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS api_keys_cache_version_nn;

DROP TABLE IF EXISTS cache_namespace_versions;
