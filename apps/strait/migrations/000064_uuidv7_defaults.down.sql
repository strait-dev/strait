-- 000064: Revert uuidv7() defaults
DO $$
BEGIN
  ALTER TABLE audit_events       ALTER COLUMN id DROP DEFAULT;
  ALTER TABLE run_events         ALTER COLUMN id DROP DEFAULT;
  ALTER TABLE webhook_deliveries ALTER COLUMN id DROP DEFAULT;
  ALTER TABLE event_triggers     ALTER COLUMN id DROP DEFAULT;
  ALTER TABLE workflow_step_runs ALTER COLUMN id DROP DEFAULT;
  ALTER TABLE workflow_runs      ALTER COLUMN id DROP DEFAULT;
  ALTER TABLE workflows          ALTER COLUMN id DROP DEFAULT;
  ALTER TABLE job_runs           ALTER COLUMN id DROP DEFAULT;
  ALTER TABLE jobs               ALTER COLUMN id DROP DEFAULT;
  DROP EXTENSION IF EXISTS pg_uuidv7;
EXCEPTION WHEN OTHERS THEN
  RAISE NOTICE 'pg_uuidv7 revert skipped: %', SQLERRM;
END;
$$;
