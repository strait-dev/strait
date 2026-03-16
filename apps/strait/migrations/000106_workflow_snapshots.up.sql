-- Workflow snapshots capture the full workflow definition (metadata + all steps)
-- as JSONB at the time a workflow run starts. This makes in-flight runs immune
-- to live workflow_steps table changes.

CREATE TABLE IF NOT EXISTS workflow_snapshots (
    id          TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    version_id  TEXT NOT NULL DEFAULT '',
    version     INT  NOT NULL DEFAULT 1,
    definition  JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_snapshots_wf_version
    ON workflow_snapshots (workflow_id, version_id)
    WHERE version_id != '';

CREATE INDEX IF NOT EXISTS idx_workflow_snapshots_workflow_id
    ON workflow_snapshots (workflow_id);

-- Link workflow runs to their snapshot (nullable for backward compat with pre-snapshot runs).
ALTER TABLE workflow_runs ADD COLUMN IF NOT EXISTS workflow_snapshot_id TEXT REFERENCES workflow_snapshots(id);
