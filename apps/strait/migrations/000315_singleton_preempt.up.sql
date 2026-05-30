-- Opt-in holder preemption for singleton execution: under the queue policy, a
-- higher-priority newcomer may cancel the running lower-priority holder and take
-- its place. The bounded 0-10 priority range caps preemption chains (each one
-- requires a strictly higher priority), so no separate time-based starvation
-- guard is needed.
--
-- Adding a NOT NULL boolean with a constant default is a metadata-only change in
-- PostgreSQL 11+; it does not rewrite these tables or hold a long lock. The
-- per-statement safety-ok annotations below acknowledge the add-column-not-null
-- linter rule for exactly that reason.

-- safety-ok: constant-default add is metadata-only in PostgreSQL 11+ (no rewrite).
ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS singleton_preempt_higher_priority BOOLEAN NOT NULL DEFAULT FALSE;

-- safety-ok: constant-default add is metadata-only in PostgreSQL 11+ (no rewrite).
ALTER TABLE job_versions
    ADD COLUMN IF NOT EXISTS singleton_preempt_higher_priority BOOLEAN NOT NULL DEFAULT FALSE;

-- safety-ok: constant-default add is metadata-only in PostgreSQL 11+ (no rewrite).
ALTER TABLE workflows
    ADD COLUMN IF NOT EXISTS singleton_preempt_higher_priority BOOLEAN NOT NULL DEFAULT FALSE;

-- safety-ok: constant-default add is metadata-only in PostgreSQL 11+ (no rewrite).
ALTER TABLE workflow_versions
    ADD COLUMN IF NOT EXISTS singleton_preempt_higher_priority BOOLEAN NOT NULL DEFAULT FALSE;
