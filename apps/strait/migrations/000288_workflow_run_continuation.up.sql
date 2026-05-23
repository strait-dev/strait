-- Workflow-level continue-as-new lineage. A successor run links back to its
-- predecessor via continued_from_workflow_run_id, and the predecessor records
-- its successor via continued_to_workflow_run_id, forming a navigable chain.
-- lineage_depth carries the chain depth to guard against runaway chains.
-- ON DELETE SET NULL (rather than the bare TEXT columns used for retry_of_run_id
-- and parent_workflow_run_id) so the retention reaper can delete any run in a
-- continuation chain: deleting one neighbor nulls the dangling lineage pointer on
-- the survivor instead of raising a foreign-key violation that would abort the
-- whole batch DELETE and stall retention.
ALTER TABLE workflow_runs ADD COLUMN continued_from_workflow_run_id TEXT NULL REFERENCES workflow_runs(id) ON DELETE SET NULL;
ALTER TABLE workflow_runs ADD COLUMN continued_to_workflow_run_id TEXT NULL REFERENCES workflow_runs(id) ON DELETE SET NULL;

-- safety-ok: INT NOT NULL DEFAULT 0 uses a constant default; PostgreSQL 11+ stores
-- it as catalog metadata and does not rewrite existing rows, so there is no table rewrite.
ALTER TABLE workflow_runs ADD COLUMN lineage_depth INT NOT NULL DEFAULT 0;

-- safety-ok: golang-migrate runs this migration inside a transaction where CONCURRENTLY
-- is not viable; this partial index only tracks the small set of continued successors.
CREATE INDEX idx_workflow_runs_continued_from
    ON workflow_runs (continued_from_workflow_run_id)
    WHERE continued_from_workflow_run_id IS NOT NULL;
