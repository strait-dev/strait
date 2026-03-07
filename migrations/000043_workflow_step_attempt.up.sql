-- Add attempt column to workflow_step_runs
ALTER TABLE workflow_step_runs ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 1;
