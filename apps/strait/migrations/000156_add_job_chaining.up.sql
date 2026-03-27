-- Job chaining: on_complete triggers a downstream job, on_failure triggers a job or workflow.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS on_complete_trigger_job TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS on_failure_trigger_job TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS on_failure_trigger_workflow TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS on_failure_payload_mapping JSONB;
