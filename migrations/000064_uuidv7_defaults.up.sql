-- 000064: Native uuidv7() defaults for primary keys (Plan 3.4)
-- Requires pg_uuidv7 extension on PostgreSQL 18.
-- Application code always passes explicit IDs; DEFAULT only fires when no value provided.
-- Gracefully skips when the extension is not available (CI, testcontainers).

DO $$
BEGIN
  CREATE EXTENSION IF NOT EXISTS pg_uuidv7;

  ALTER TABLE jobs               ALTER COLUMN id SET DEFAULT uuidv7();
  ALTER TABLE job_runs           ALTER COLUMN id SET DEFAULT uuidv7();
  ALTER TABLE workflows          ALTER COLUMN id SET DEFAULT uuidv7();
  ALTER TABLE workflow_runs      ALTER COLUMN id SET DEFAULT uuidv7();
  ALTER TABLE workflow_step_runs ALTER COLUMN id SET DEFAULT uuidv7();
  ALTER TABLE event_triggers     ALTER COLUMN id SET DEFAULT uuidv7();
  ALTER TABLE webhook_deliveries ALTER COLUMN id SET DEFAULT uuidv7();
  ALTER TABLE run_events         ALTER COLUMN id SET DEFAULT uuidv7();
  ALTER TABLE audit_events       ALTER COLUMN id SET DEFAULT uuidv7();
EXCEPTION WHEN OTHERS THEN
  RAISE NOTICE 'pg_uuidv7 not available, skipping uuidv7() defaults: %', SQLERRM;
END;
$$;
