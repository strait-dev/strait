-- 000063: Autovacuum tuning for high-churn tables (Plan 3.3)
-- Default autovacuum triggers at 20% dead tuple ratio. job_runs generates
-- dead tuples on every state transition (5-13 per run). At high throughput,
-- dead tuple accumulation outpaces default vacuum.

ALTER TABLE job_runs SET (
  autovacuum_vacuum_scale_factor       = 0.01,
  autovacuum_analyze_scale_factor      = 0.005,
  autovacuum_vacuum_cost_delay         = 2,
  autovacuum_vacuum_insert_scale_factor = 0.01
);

ALTER TABLE webhook_deliveries SET (
  autovacuum_vacuum_scale_factor       = 0.02,
  autovacuum_analyze_scale_factor      = 0.01,
  autovacuum_vacuum_cost_delay         = 5
);

ALTER TABLE run_events SET (
  autovacuum_vacuum_scale_factor       = 0.05,
  autovacuum_analyze_scale_factor      = 0.02,
  autovacuum_vacuum_cost_delay         = 10
);
