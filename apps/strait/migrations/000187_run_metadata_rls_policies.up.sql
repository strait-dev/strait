-- Tenant-isolation RLS policies on run_iterations and run_events.
--
-- Neither table has a project_id column; both route through job_runs
-- via the EXISTS-through-parent pattern established for
-- job_slo_evaluations in migration 000182.
--
-- Why this matters: run_events carries arbitrary error data payloads
-- and streaming message chunks, and run_iterations carries agent
-- planner descriptions. Cross-tenant reads of either table leak
-- per-run metadata. Before this migration the only guard was
-- application-layer filtering by run_id.
--
-- Phase F3 of the agents hardening work.

ALTER TABLE run_iterations ENABLE ROW LEVEL SECURITY;
ALTER TABLE run_iterations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON run_iterations
    USING (EXISTS (
        SELECT 1 FROM job_runs jr
        WHERE jr.id = run_id
          AND (jr.project_id = current_setting('app.current_project_id', true)
               OR current_setting('app.current_project_id', true) = '')
    ));

ALTER TABLE run_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE run_events FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON run_events
    USING (EXISTS (
        SELECT 1 FROM job_runs jr
        WHERE jr.id = run_id
          AND (jr.project_id = current_setting('app.current_project_id', true)
               OR current_setting('app.current_project_id', true) = '')
    ));
