-- Add human-readable version IDs (nanoid) alongside the integer version counter.
ALTER TABLE jobs ADD COLUMN version_id TEXT;
ALTER TABLE workflows ADD COLUMN version_id TEXT;
ALTER TABLE job_versions ADD COLUMN version_id TEXT;
ALTER TABLE workflow_versions ADD COLUMN version_id TEXT;
ALTER TABLE job_runs ADD COLUMN job_version_id TEXT;
ALTER TABLE workflow_runs ADD COLUMN workflow_version_id TEXT;

CREATE UNIQUE INDEX idx_jobs_version_id ON jobs(version_id) WHERE version_id IS NOT NULL;
CREATE UNIQUE INDEX idx_workflows_version_id ON workflows(version_id) WHERE version_id IS NOT NULL;
