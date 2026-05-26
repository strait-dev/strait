CREATE TABLE IF NOT EXISTS cache_namespace_versions (
    namespace TEXT NOT NULL,
    cache_key TEXT NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (namespace, cache_key)
);

-- safety-ok: SET NOT NULL is preceded by backfill plus validated CHECK constraint in this helper.
CREATE OR REPLACE FUNCTION ensure_cache_version_not_null(table_name TEXT) RETURNS void AS $$
BEGIN
    EXECUTE format('UPDATE %I SET cache_version = 1 WHERE cache_version IS NULL', table_name);
    EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I CHECK (cache_version IS NOT NULL) NOT VALID', table_name, table_name || '_cache_version_nn');
    EXECUTE format('ALTER TABLE %I VALIDATE CONSTRAINT %I', table_name, table_name || '_cache_version_nn');
    EXECUTE format('ALTER TABLE %I ALTER COLUMN cache_version SET NOT NULL', table_name);
END;
$$ LANGUAGE plpgsql;

SELECT ensure_cache_version_not_null('api_keys');
SELECT ensure_cache_version_not_null('project_roles');
SELECT ensure_cache_version_not_null('project_member_roles');
SELECT ensure_cache_version_not_null('resource_policies');
SELECT ensure_cache_version_not_null('tag_policies');
SELECT ensure_cache_version_not_null('project_quotas');
SELECT ensure_cache_version_not_null('organization_subscriptions');
SELECT ensure_cache_version_not_null('jobs');
SELECT ensure_cache_version_not_null('job_dependencies');
SELECT ensure_cache_version_not_null('job_runs');
SELECT ensure_cache_version_not_null('job_runs_history');
SELECT ensure_cache_version_not_null('workflow_runs');
SELECT ensure_cache_version_not_null('workflow_step_runs');

DROP FUNCTION IF EXISTS ensure_cache_version_not_null(TEXT);
