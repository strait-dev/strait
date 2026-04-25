-- webhook_deliveries was created in 000009 without a project_id column and
-- never gained one. It references jobs/runs/workflows/event_triggers via
-- nullable FKs, but there is no direct tenant key, so RLS could not be
-- applied. This migration adds project_id, backfills it from whichever
-- FK is populated, enables RLS, and takes the same FORCE + tenant policy
-- shape the sibling tables got in 000182.
--
-- Backfill precedence:
--   1. run_id          -> job_runs.project_id
--   2. workflow_run_id -> workflow_runs.project_id
--   3. event_trigger_id -> event_triggers.project_id
--   4. subscription_id -> webhook_subscriptions.project_id
--   5. job_id          -> jobs.project_id
--
-- Rows where none of the FKs resolve get tagged with the sentinel
-- '__orphaned__' so the reaper can find and delete them in a later
-- cleanup pass.

ALTER TABLE webhook_deliveries ADD COLUMN IF NOT EXISTS project_id TEXT;

UPDATE webhook_deliveries wd
SET project_id = COALESCE(
    (SELECT jr.project_id FROM job_runs             jr WHERE jr.id = wd.run_id),
    (SELECT wr.project_id FROM workflow_runs        wr WHERE wr.id = wd.workflow_run_id),
    (SELECT et.project_id FROM event_triggers       et WHERE et.id = wd.event_trigger_id),
    (SELECT ws.project_id FROM webhook_subscriptions ws WHERE ws.id = wd.subscription_id),
    (SELECT j.project_id  FROM jobs                 j  WHERE j.id  = wd.job_id)
)
WHERE project_id IS NULL;

UPDATE webhook_deliveries SET project_id = '__orphaned__' WHERE project_id IS NULL;

ALTER TABLE webhook_deliveries ALTER COLUMN project_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_project_created
    ON webhook_deliveries (project_id, created_at DESC);

ALTER TABLE webhook_deliveries ENABLE ROW LEVEL SECURITY;
ALTER TABLE webhook_deliveries FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON webhook_deliveries
    USING (project_id = current_setting('app.current_project_id', true)
           OR current_setting('app.current_project_id', true) = '');
