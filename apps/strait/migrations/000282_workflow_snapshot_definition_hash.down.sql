WITH ranked AS (
    SELECT
        id,
        workflow_id,
        version_id,
        FIRST_VALUE(id) OVER (
            PARTITION BY workflow_id, version_id
            ORDER BY created_at ASC, id ASC
        ) AS keep_id,
        ROW_NUMBER() OVER (
            PARTITION BY workflow_id, version_id
            ORDER BY created_at ASC, id ASC
        ) AS rn
    FROM workflow_snapshots
    WHERE version_id != ''
),
duplicates AS (
    SELECT id, keep_id
    FROM ranked
    WHERE rn > 1
)
UPDATE workflow_runs wr
SET workflow_snapshot_id = duplicates.keep_id
FROM duplicates
WHERE wr.workflow_snapshot_id = duplicates.id;

WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY workflow_id, version_id
            ORDER BY created_at ASC, id ASC
        ) AS rn
    FROM workflow_snapshots
    WHERE version_id != ''
)
DELETE FROM workflow_snapshots ws
USING ranked
WHERE ws.id = ranked.id
  AND ranked.rn > 1;

DROP INDEX IF EXISTS idx_workflow_snapshots_wf_version_definition;

ALTER TABLE workflow_snapshots
    DROP COLUMN IF EXISTS definition_hash;

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_snapshots_wf_version
    ON workflow_snapshots (workflow_id, version_id)
    WHERE version_id != '';
