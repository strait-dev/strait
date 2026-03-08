-- Composite index for the delayed job poller query:
-- WHERE status = 'delayed' AND scheduled_at <= NOW() ORDER BY scheduled_at ASC
CREATE INDEX IF NOT EXISTS idx_job_runs_delayed_scheduled
    ON job_runs(scheduled_at ASC)
    WHERE status = 'delayed';

-- Composite index for listing jobs by project with enabled filter:
-- WHERE project_id = $1 (with enabled filter and created_at ordering)
CREATE INDEX IF NOT EXISTS idx_jobs_project_enabled_created
    ON jobs(project_id, enabled, created_at DESC);

-- Composite index for listing workflow runs by project with created_at ordering:
-- WHERE project_id = $1 ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_workflow_runs_project_created
    ON workflow_runs(project_id, created_at DESC);
