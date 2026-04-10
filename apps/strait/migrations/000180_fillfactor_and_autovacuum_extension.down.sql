ALTER TABLE run_usage RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);
ALTER TABLE workflow_step_runs RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);
ALTER TABLE workflow_runs RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);

ALTER TABLE webhook_deliveries RESET (fillfactor);
ALTER TABLE workflow_step_runs RESET (fillfactor);
ALTER TABLE workflow_runs RESET (fillfactor);
ALTER TABLE job_runs RESET (fillfactor);
