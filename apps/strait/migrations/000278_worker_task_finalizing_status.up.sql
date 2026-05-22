ALTER TABLE worker_tasks DROP CONSTRAINT IF EXISTS worker_tasks_status_check;

ALTER TABLE worker_tasks
    ADD CONSTRAINT worker_tasks_status_check
    CHECK (status IN ('assigned', 'accepted', 'result_received', 'finalizing', 'completed', 'failed'));
