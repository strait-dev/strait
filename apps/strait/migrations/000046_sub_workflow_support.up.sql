-- Add sub-workflow support columns to workflow_steps
ALTER TABLE workflow_steps ADD COLUMN sub_workflow_id TEXT;
ALTER TABLE workflow_steps ADD COLUMN max_nesting_depth INTEGER NOT NULL DEFAULT 0;

-- Add sub-workflow support columns to workflow_version_steps
ALTER TABLE workflow_version_steps ADD COLUMN sub_workflow_id TEXT;
ALTER TABLE workflow_version_steps ADD COLUMN max_nesting_depth INTEGER NOT NULL DEFAULT 0;

-- Add parent workflow run tracking to workflow_runs
ALTER TABLE workflow_runs ADD COLUMN parent_workflow_run_id TEXT;

-- Index for looking up child workflow runs by parent
CREATE INDEX idx_workflow_runs_parent_workflow_run_id ON workflow_runs(parent_workflow_run_id) WHERE parent_workflow_run_id IS NOT NULL;
