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
ALTER TABLE job_runs           SET (fillfactor = 85);
ALTER TABLE workflow_runs      SET (fillfactor = 85);
ALTER TABLE workflow_step_runs SET (fillfactor = 85);
ALTER TABLE webhook_deliveries SET (fillfactor = 85);

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
