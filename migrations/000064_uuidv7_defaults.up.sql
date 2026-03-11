-- 000064: Native uuidv7() defaults for primary keys (Plan 3.4)
-- Requires pg_uuidv7 extension on PostgreSQL 18.
-- Application code may still pass explicit IDs; DEFAULT only fires when no value provided.

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
