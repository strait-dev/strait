-- safety-ok: workflow_snapshots is version metadata; Postgres 11+ stores this
-- constant default metadata-only, and the hash must be non-null before the
-- replacement uniqueness constraint is installed.
ALTER TABLE workflow_snapshots
    ADD COLUMN IF NOT EXISTS definition_hash TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS idx_workflow_snapshots_wf_version;

-- safety-ok: replaces the existing workflow snapshot uniqueness shape in the
-- same migration. golang-migrate wraps migrations in a transaction, so
-- CONCURRENTLY is not viable here.
CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_snapshots_wf_version_definition
    ON workflow_snapshots (workflow_id, version_id, definition_hash)
    WHERE version_id != '';
