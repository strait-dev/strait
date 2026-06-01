-- job_run_state carries the high-churn mutable fields split out of job_runs.
-- Leave page headroom for HOT-eligible updates and make autovacuum react
-- quickly because queued->dequeued->executing->terminal transitions still
-- produce dead tuples even when the fat ledger stays immutable.
ALTER TABLE job_run_state SET (
    fillfactor = 70,
    autovacuum_vacuum_threshold = 50,
    autovacuum_vacuum_scale_factor = 0.005,
    autovacuum_vacuum_cost_delay = 0,
    autovacuum_vacuum_cost_limit = 2000,
    autovacuum_analyze_threshold = 50,
    autovacuum_analyze_scale_factor = 0.005
);
