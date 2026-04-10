-- Lower fillfactor on high-update tables so that HOT (heap-only tuple)
-- updates have room to stay in-page. Without free space on the target
-- page, an UPDATE must write a new heap tuple and update every index,
-- even if the updated column is not indexed. 85% leaves 15% for in-page
-- updates, which is sufficient for the status/timestamp churn these
-- tables see without materially increasing storage.
--
-- Note: fillfactor only applies to newly written pages. Existing pages
-- keep their current layout until the next vacuum/rewrite. This change
-- prevents future bloat rather than repacking existing bloat.

ALTER TABLE workflow_runs      SET (fillfactor = 85);
ALTER TABLE workflow_step_runs SET (fillfactor = 85);
ALTER TABLE webhook_deliveries SET (fillfactor = 85);

-- job_runs is partitioned (migration 000066), and Postgres does not
-- allow storage parameters like fillfactor on a partitioned parent.
-- Apply fillfactor to every existing partition instead. New partitions
-- created by pg_partman after this migration will inherit the default
-- fillfactor; the maintenance loop can re-run this block to catch up
-- if that becomes a concern.
DO $$
DECLARE
    partition_name TEXT;
BEGIN
    FOR partition_name IN
        SELECT c.relname
        FROM pg_class c
        JOIN pg_inherits i ON i.inhrelid = c.oid
        JOIN pg_class p   ON p.oid       = i.inhparent
        WHERE p.relname = 'job_runs'
          AND c.relkind = 'r'
    LOOP
        EXECUTE format('ALTER TABLE %I SET (fillfactor = 85)', partition_name);
    END LOOP;
END $$;

-- Extend the autovacuum tuning from 000063 to additional update-heavy
-- tables that were not covered there. workflow_runs and workflow_step_runs
-- see the same status-transition churn pattern as job_runs, and run_usage
-- receives bulk inserts + occasional updates during cost reconciliation.
-- Values mirror the existing webhook_deliveries tuning from 000063 to
-- keep the policy consistent.
ALTER TABLE workflow_runs SET (
    autovacuum_vacuum_scale_factor  = 0.02,
    autovacuum_analyze_scale_factor = 0.01
);

ALTER TABLE workflow_step_runs SET (
    autovacuum_vacuum_scale_factor  = 0.02,
    autovacuum_analyze_scale_factor = 0.01
);

ALTER TABLE run_usage SET (
    autovacuum_vacuum_scale_factor  = 0.05,
    autovacuum_analyze_scale_factor = 0.02
);
