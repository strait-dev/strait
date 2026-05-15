ALTER TABLE job_active_counts RESET (
    fillfactor,
    autovacuum_vacuum_scale_factor,
    autovacuum_vacuum_cost_limit,
    autovacuum_analyze_scale_factor
);
