ALTER TABLE workflow_snapshots
    ADD COLUMN IF NOT EXISTS definition_hash TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_workflow_snapshots_wf_version;

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_snapshots_wf_version_definition
    ON workflow_snapshots (workflow_id, version_id, definition_hash)
    WHERE version_id != '';
