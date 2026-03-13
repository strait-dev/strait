DROP TRIGGER IF EXISTS trg_job_runs_queue_wake_notify ON job_runs;
DROP FUNCTION IF EXISTS notify_queue_wake();
