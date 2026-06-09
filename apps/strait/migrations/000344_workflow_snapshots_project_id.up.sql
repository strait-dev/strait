-- Add tenant scoping to workflow_snapshots. The table previously had no
-- project_id column, so snapshot lookups were filtered by id/workflow_id alone
-- and relied entirely on callers passing a server-derived id. Adding project_id
-- lets the store enforce tenant isolation directly, in line with the other
-- project-scoped tables.

-- Nullable add (no NOT NULL/DEFAULT) keeps this a fast metadata-only change and
-- avoids a full-table rewrite. New snapshots always set project_id at insert
-- time; existing rows are backfilled below. Orphaned snapshots whose workflow no
-- longer exists keep project_id NULL and simply never match a project-scoped
-- lookup, which is the safe outcome.
ALTER TABLE workflow_snapshots
    ADD COLUMN IF NOT EXISTS project_id TEXT;

-- Backfill project_id from the owning workflow for existing snapshots.
UPDATE workflow_snapshots s
SET project_id = w.project_id
FROM workflows w
WHERE s.workflow_id = w.id
  AND s.project_id IS NULL;
