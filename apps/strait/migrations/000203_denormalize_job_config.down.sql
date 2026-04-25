DROP TRIGGER IF EXISTS jobs_fanout_config_trg ON jobs;
DROP FUNCTION IF EXISTS fanout_job_config_to_runs();

ALTER TABLE job_runs
    DROP COLUMN IF EXISTS job_max_concurrency_per_key,
    DROP COLUMN IF EXISTS job_max_concurrency,
    DROP COLUMN IF EXISTS job_paused,
    DROP COLUMN IF EXISTS job_enabled;
