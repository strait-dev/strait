ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE project_roles ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE project_member_roles ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE resource_policies ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE tag_policies ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE project_quotas ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE organization_subscriptions ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE job_dependencies ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE job_runs_history ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE workflow_step_runs ADD COLUMN IF NOT EXISTS cache_version BIGINT NOT NULL DEFAULT 1;

CREATE OR REPLACE FUNCTION bump_cache_version() RETURNS trigger AS $$
BEGIN
	NEW.cache_version := COALESCE(OLD.cache_version, 0) + 1;
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS cache_version_bump ON api_keys;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON api_keys
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON project_roles;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON project_roles
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON project_member_roles;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON project_member_roles
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON resource_policies;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON resource_policies
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON tag_policies;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON tag_policies
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON project_quotas;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON project_quotas
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON organization_subscriptions;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON organization_subscriptions
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON jobs;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON jobs
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON job_dependencies;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON job_dependencies
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON job_runs;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON job_runs
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON workflow_runs;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON workflow_runs
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();

DROP TRIGGER IF EXISTS cache_version_bump ON workflow_step_runs;
CREATE TRIGGER cache_version_bump
	BEFORE UPDATE ON workflow_step_runs
	FOR EACH ROW EXECUTE FUNCTION bump_cache_version();
