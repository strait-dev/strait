-- Drop indexes that are made redundant by existing composite or unique indexes.
-- Each dropped index has a broader covering index already present; keeping
-- both wastes write overhead with no read benefit.

-- Strait tables
-- idx_jobs_project_id(project_id) is covered by UNIQUE(project_id, slug)
DROP INDEX IF EXISTS idx_jobs_project_id;

-- idx_workflow_runs_workflow_id(workflow_id) is a prefix of
-- any composite index on workflow_runs that starts with workflow_id
DROP INDEX IF EXISTS idx_workflow_runs_workflow_id;

-- idx_step_runs_workflow_run(workflow_run_id) is a prefix of
-- idx_step_runs_workflow_run_status(workflow_run_id, status)
DROP INDEX IF EXISTS idx_step_runs_workflow_run;

-- idx_run_usage_run_id(run_id) is a prefix of
-- idx_run_usage_run_id_created_at(run_id, created_at DESC)
DROP INDEX IF EXISTS idx_run_usage_run_id;

-- idx_pricing_catalog_lookup is covered by UNIQUE(provider, model, effective_from)
DROP INDEX IF EXISTS idx_pricing_catalog_lookup;

-- idx_audit_events_resource_lookup is made redundant by a broader covering index
DROP INDEX IF EXISTS idx_audit_events_resource_lookup;

-- Shared-database tables (Sequin CDC)
DROP INDEX IF EXISTS wal_pipelines_replication_slot_id_index;
DROP INDEX IF EXISTS transforms_account_id_index;
DROP INDEX IF EXISTS api_tokens_account_id_index;
DROP INDEX IF EXISTS allocated_bastion_ports_account_id_index;
DROP INDEX IF EXISTS wal_events_wal_pipeline_id_commit_lsn_index;
DROP INDEX IF EXISTS consumer_records_consumer_id_index;
DROP INDEX IF EXISTS consumer_messages_consumer_id_index;
DROP INDEX IF EXISTS consumer_events_consumer_id_not_visible_until_index;
DROP INDEX IF EXISTS consumer_events_9_consumer_id_not_visible_until_idx;
DROP INDEX IF EXISTS consumer_events_8_consumer_id_not_visible_until_idx;
DROP INDEX IF EXISTS consumer_events_16_consumer_id_not_visible_until_idx;
DROP INDEX IF EXISTS consumer_events_15_consumer_id_not_visible_until_idx;
DROP INDEX IF EXISTS consumer_events_14_consumer_id_not_visible_until_idx;
