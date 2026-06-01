ALTER TABLE job_run_state RESET (
    fillfactor,
    autovacuum_vacuum_threshold,
    autovacuum_vacuum_scale_factor,
    autovacuum_vacuum_cost_delay,
    autovacuum_vacuum_cost_limit,
    autovacuum_analyze_threshold,
    autovacuum_analyze_scale_factor
);
