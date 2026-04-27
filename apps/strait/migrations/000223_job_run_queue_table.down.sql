DROP TRIGGER IF EXISTS trg_jobs_fanout_queue ON jobs;
DROP FUNCTION IF EXISTS trg_jobs_fanout_to_queue();
DROP TABLE IF EXISTS job_run_queue;
