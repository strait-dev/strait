-- Increase autovacuum_vacuum_cost_limit on existing job_runs partitions
-- so vacuum does 5x more work per cycle before napping. The system-wide
-- default of 200 is too conservative for a high-churn queue table.
--
-- Cannot set storage parameters on the partitioned parent table directly;
-- applied per-partition instead. The partition_tuner applies this at
-- runtime for hot partitions, but this migration catches any existing
-- partitions that have not yet been tuned.
DO $$
DECLARE
    part TEXT;
BEGIN
    FOR part IN
        SELECT c.relname
        FROM pg_class c
        JOIN pg_inherits i ON i.inhrelid = c.oid
        JOIN pg_class p ON p.oid = i.inhparent
        WHERE p.relname = 'job_runs'
    LOOP
        EXECUTE format(
            'ALTER TABLE %I SET (autovacuum_vacuum_cost_limit = 1000)',
            part
        );
    END LOOP;
END
$$;
