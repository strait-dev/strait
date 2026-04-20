DROP TRIGGER IF EXISTS job_runs_dlq_counts_trg ON job_runs;
DROP FUNCTION IF EXISTS dlq_counts_apply();
DROP TABLE IF EXISTS dlq_counts;
