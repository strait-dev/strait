DROP INDEX IF EXISTS idx_job_run_queue_dequeue;

CREATE INDEX idx_job_run_queue_dequeue
  ON job_run_queue (priority DESC, created_at ASC);
