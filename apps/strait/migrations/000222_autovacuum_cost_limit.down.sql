-- Reset to system default (200).
ALTER TABLE job_runs RESET (
  autovacuum_vacuum_cost_limit
);
