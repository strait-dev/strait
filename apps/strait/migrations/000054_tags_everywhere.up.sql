-- Tags on all core entities for filtering and organization.
ALTER TABLE workflows ADD COLUMN tags JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE workflow_runs ADD COLUMN tags JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE job_runs ADD COLUMN tags JSONB NOT NULL DEFAULT '{}'::jsonb;

-- GIN indexes for efficient tag queries.
CREATE INDEX idx_workflows_tags ON workflows USING gin(tags);
CREATE INDEX idx_workflow_runs_tags ON workflow_runs USING gin(tags);
CREATE INDEX idx_job_runs_tags ON job_runs USING gin(tags);
CREATE INDEX idx_jobs_tags ON jobs USING gin(tags);
