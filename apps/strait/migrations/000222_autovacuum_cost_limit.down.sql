-- Reset autovacuum_vacuum_cost_limit on all job_runs partitions.
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
            'ALTER TABLE %I RESET (autovacuum_vacuum_cost_limit)',
            part
        );
    END LOOP;
END
$$;
