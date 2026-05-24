-- Wave 2 Phase 2: collapse the partial-on-terminal-status idempotency
-- index into a single non-partial-on-status index keyed only on the
-- lookup columns.
--
-- Background:
--   idx_runs_idempotency_terminal (000155) covered (job_id, idempotency_key,
--   finished_at) WHERE idempotency_key IS NOT NULL AND status IN (six terminal
--   statuses). Because the partial predicate is on a column that changes
--   on every run completion, every terminal status flip inserts a row into
--   the index. With ~all SDK-triggered runs carrying an idempotency key,
--   this is per-run write amplification on a partitioned hot table.
--
--   GetRunByIdempotencyKey reads both the non-terminal and the
--   terminal-within-24h branch. A single non-partial-on-status index keyed
--   on (job_id, idempotency_key) serves both branches and is written
--   exactly once per run (on initial CreateRun), never on status
--   transitions.
--
--   The finished_at column is removed from the index columns: the
--   24h-window check in the read query is satisfied by a row fetch after
--   the (job_id, idempotency_key) lookup narrows results to typically <=2
--   rows.
--
--   The index name idx_runs_idempotency is free: it was created in 000007,
--   replaced in 000026, and dropped in 000073 in favor of CTE-based dedup
--   inside CreateRun. Reusing the name keeps the symbol surface small.

-- safety-ok: job_runs is partitioned by RANGE(created_at); CREATE INDEX
-- CONCURRENTLY on a partitioned parent is not supported by the project's
-- golang-migrate runner (which wraps each .up.sql file in a transaction).
-- DROP INDEX cascades to all per-partition child indexes and takes only a
-- brief ACCESS EXCLUSIVE lock per partition. Matches the swap pattern used
-- in 000196 for the audit_events anchor uniqueness index.
DROP INDEX IF EXISTS idx_runs_idempotency_terminal;

-- safety-ok: same constraint as the DROP above. The replacement index is
-- required: there is currently no other index on job_runs covering the
-- (job_id, idempotency_key) lookup (000073 dropped the prior active-only
-- partial), and both CreateRun's dedup CTE and GetRunByIdempotencyKey's
-- read path scan partitions without it. The CREATE locks each partition
-- briefly while building per-partition btrees; partial-on-NOT-NULL keeps
-- the index size proportional to idempotent-key traffic only.
CREATE INDEX IF NOT EXISTS idx_runs_idempotency
    ON job_runs (job_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
