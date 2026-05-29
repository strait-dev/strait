DROP INDEX IF EXISTS idx_workflow_runs_continued_from;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS lineage_depth;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS continued_to_workflow_run_id;
ALTER TABLE workflow_runs DROP COLUMN IF EXISTS continued_from_workflow_run_id;
