-- Track compensation progress on workflow runs.
-- compensation_status: none | pending | running | completed | partial | failed
ALTER TABLE workflow_runs ADD COLUMN compensation_status TEXT NOT NULL DEFAULT 'none';
ALTER TABLE workflow_runs ADD COLUMN compensation_steps_total INT NOT NULL DEFAULT 0;
ALTER TABLE workflow_runs ADD COLUMN compensation_steps_completed INT NOT NULL DEFAULT 0;

-- Index for querying runs that need compensation attention
CREATE INDEX idx_workflow_runs_compensation ON workflow_runs (compensation_status)
    WHERE compensation_status NOT IN ('none', 'completed');
