ALTER TABLE jobs DROP COLUMN IF EXISTS on_complete_trigger_job;
ALTER TABLE jobs DROP COLUMN IF EXISTS on_failure_trigger_job;
ALTER TABLE jobs DROP COLUMN IF EXISTS on_failure_trigger_workflow;
ALTER TABLE jobs DROP COLUMN IF EXISTS on_failure_payload_mapping;
