-- ListAPIKeysByProject in internal/store/api_keys.go runs
--   WHERE project_id = $1 AND revoked_at IS NULL ORDER BY created_at DESC
-- The existing idx_api_keys_project_id is a single-column non-partial
-- index, so the query walks every key for the project (including revoked
-- ones) and sorts. This partial composite covers filter + predicate + sort.
CREATE INDEX IF NOT EXISTS idx_api_keys_project_active_created
    ON api_keys (project_id, created_at DESC)
    WHERE revoked_at IS NULL;

-- ListAPIKeysByOrg runs the same shape with org_id. The existing
-- idx_api_keys_org_id is partial on WHERE org_id IS NOT NULL but does
-- not filter on revoked_at.
CREATE INDEX IF NOT EXISTS idx_api_keys_org_active_created
    ON api_keys (org_id, created_at DESC)
    WHERE org_id IS NOT NULL AND revoked_at IS NULL;

-- ListEnabledLogDrains runs
--   WHERE enabled = true ORDER BY created_at DESC LIMIT 500
-- without a project filter, called on every event dispatch. The existing
-- idx_log_drains_project has leading column project_id and cannot answer
-- this unqualified query. This partial index walks enabled rows in
-- created_at DESC order directly.
CREATE INDEX IF NOT EXISTS idx_log_drains_enabled_created
    ON log_drains (created_at DESC)
    WHERE enabled = true;
