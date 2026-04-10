ALTER TABLE run_usage RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);
ALTER TABLE workflow_step_runs RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);
ALTER TABLE workflow_runs RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);

ALTER TABLE webhook_deliveries RESET (fillfactor);
ALTER TABLE workflow_step_runs RESET (fillfactor);
ALTER TABLE workflow_runs RESET (fillfactor);

DO $$
DECLARE
    partition_name TEXT;
BEGIN
    FOR partition_name IN
        SELECT c.relname
        FROM pg_class c
        JOIN pg_inherits i ON i.inhrelid = c.oid
        JOIN pg_class p   ON p.oid       = i.inhparent
        WHERE p.relname = 'job_runs'
          AND c.relkind = 'r'
    LOOP
        EXECUTE format('ALTER TABLE %I RESET (fillfactor)', partition_name);
    END LOOP;
END $$;
