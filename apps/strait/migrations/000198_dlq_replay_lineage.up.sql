-- Phase 3: DLQ admin recovery endpoints require tracking replay lineage so
-- operators can see that a dead-lettered run was superseded by a new one.
-- The column is plain-nullable UUID without a foreign-key constraint:
-- job_runs is partitioned by month (PARTITION BY RANGE on created_at), and
-- Postgres does not support a self-referential FK whose target key is just
-- (id) on a partitioned table — the partition column would have to be part
-- of the referenced key. Lineage integrity is enforced at the application
-- layer in store.MarkRunReplayed (the caller always sets this column to an
-- id the caller just wrote inside the same transaction), and partition
-- drops are tolerated because a broken pointer reads as "unknown ancestor"
-- rather than corrupting the run row.
--
-- The companion index moved to migration 199 so it can be created
-- CONCURRENTLY outside any implicit DDL transaction, satisfying the
-- migration-lint rule that bans online CREATE INDEX without CONCURRENTLY.

ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS replayed_run_id UUID NULL;
UPDATE schema_version SET version = 198, updated_at = NOW();
