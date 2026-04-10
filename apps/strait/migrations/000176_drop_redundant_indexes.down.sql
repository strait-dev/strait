-- Restore the redundant indexes removed in the up migration.

-- Strait tables
CREATE INDEX IF NOT EXISTS idx_jobs_project_id ON jobs (project_id);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_workflow_id ON workflow_runs (workflow_id);
CREATE INDEX IF NOT EXISTS idx_step_runs_workflow_run ON workflow_step_runs (workflow_run_id);
CREATE INDEX IF NOT EXISTS idx_run_usage_run_id ON run_usage (run_id);
CREATE INDEX IF NOT EXISTS idx_pricing_catalog_lookup ON pricing_catalog (provider, model, active, effective_from DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_resource_lookup ON audit_events (resource_type, resource_id);

-- Shared-database tables (Sequin CDC)
CREATE INDEX IF NOT EXISTS wal_pipelines_replication_slot_id_index ON wal_pipelines (replication_slot_id);
CREATE INDEX IF NOT EXISTS transforms_account_id_index ON functions (account_id);
CREATE INDEX IF NOT EXISTS api_tokens_account_id_index ON api_tokens (account_id);
CREATE INDEX IF NOT EXISTS allocated_bastion_ports_account_id_index ON allocated_bastion_ports (account_id);
CREATE INDEX IF NOT EXISTS wal_events_wal_pipeline_id_commit_lsn_index ON wal_events (wal_pipeline_id, commit_lsn);
CREATE INDEX IF NOT EXISTS consumer_records_consumer_id_index ON consumer_records (consumer_id);
CREATE INDEX IF NOT EXISTS consumer_messages_consumer_id_index ON consumer_messages (consumer_id);
CREATE INDEX IF NOT EXISTS consumer_events_consumer_id_not_visible_until_index ON consumer_events (consumer_id, not_visible_until);
CREATE INDEX IF NOT EXISTS consumer_events_9_consumer_id_not_visible_until_idx ON consumer_events_9 (consumer_id, not_visible_until);
CREATE INDEX IF NOT EXISTS consumer_events_8_consumer_id_not_visible_until_idx ON consumer_events_8 (consumer_id, not_visible_until);
CREATE INDEX IF NOT EXISTS consumer_events_16_consumer_id_not_visible_until_idx ON consumer_events_16 (consumer_id, not_visible_until);
CREATE INDEX IF NOT EXISTS consumer_events_15_consumer_id_not_visible_until_idx ON consumer_events_15 (consumer_id, not_visible_until);
CREATE INDEX IF NOT EXISTS consumer_events_14_consumer_id_not_visible_until_idx ON consumer_events_14 (consumer_id, not_visible_until);
