-- 000063: Revert autovacuum tuning to defaults
ALTER TABLE job_runs RESET (
  autovacuum_vacuum_scale_factor,
  autovacuum_analyze_scale_factor,
  autovacuum_vacuum_cost_delay,
  autovacuum_vacuum_insert_scale_factor
);

ALTER TABLE webhook_deliveries RESET (
  autovacuum_vacuum_scale_factor,
  autovacuum_analyze_scale_factor,
  autovacuum_vacuum_cost_delay
);

ALTER TABLE run_events RESET (
  autovacuum_vacuum_scale_factor,
  autovacuum_analyze_scale_factor,
  autovacuum_vacuum_cost_delay
);
