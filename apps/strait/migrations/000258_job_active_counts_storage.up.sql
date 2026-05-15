-- job_active_counts is the hottest counter table: every queue claim and
-- run completion bumps the row. Default fillfactor=100 forces out-of-page
-- writes on every UPDATE, defeating HOT updates and producing dead rows
-- faster than autovacuum's default settings can keep up. Tune both: leave
-- 30% free space per page so HOT can succeed, and run autovacuum sooner
-- and harder so dead tuples don't accumulate.
ALTER TABLE job_active_counts SET (
    fillfactor = 70,
    autovacuum_vacuum_scale_factor = 0.01,
    autovacuum_vacuum_cost_limit = 1000,
    autovacuum_analyze_scale_factor = 0.02
);
