UPDATE worker_tasks
SET status = 'failed',
    finished_at = COALESCE(finished_at, NOW())
WHERE status = 'result_received';

ALTER TABLE worker_tasks DROP CONSTRAINT IF EXISTS worker_tasks_status_check;

ALTER TABLE worker_tasks
    ADD CONSTRAINT worker_tasks_status_check
    CHECK (status IN ('assigned', 'accepted', 'completed', 'failed'));
