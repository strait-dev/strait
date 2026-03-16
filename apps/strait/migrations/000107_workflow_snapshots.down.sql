ALTER TABLE workflow_runs DROP COLUMN IF EXISTS workflow_snapshot_id;

DROP TABLE IF EXISTS workflow_snapshots;
