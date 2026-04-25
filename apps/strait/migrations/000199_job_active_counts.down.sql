DROP TRIGGER IF EXISTS job_runs_active_counts_trg ON job_runs;
DROP FUNCTION IF EXISTS job_active_counts_apply();
DROP TABLE IF EXISTS job_active_counts;
