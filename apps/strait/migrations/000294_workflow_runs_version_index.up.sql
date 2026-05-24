-- Index for CountActiveWorkflowRunsByVersion and ListActiveWorkflowVersions
-- (canary deployments and version management): both filter workflow_runs by
-- workflow_id over an active-status set, the former additionally by
-- workflow_version_id and the latter grouping per version. The existing
-- idx_workflow_runs_workflow_status_created leads with (workflow_id, status) and
-- cannot satisfy the version predicate or the per-version grouping from the
-- index, forcing a partial scan. The partial predicate scopes this index to
-- active runs only, keeping maintenance cheap.
--
-- workflow_runs is not partitioned, so this is built CONCURRENTLY to avoid an
-- ACCESS EXCLUSIVE lock. CONCURRENTLY cannot run inside a transaction;
-- golang-migrate handles that automatically because this is the only statement
-- in the file.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_workflow_runs_workflow_version
    ON workflow_runs (workflow_id, workflow_version_id)
    WHERE status IN ('pending', 'running', 'paused');
