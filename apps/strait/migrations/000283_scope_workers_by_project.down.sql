-- Revert worker identity to the legacy global worker-id primary key.
-- This rollback requires worker IDs to be globally unique again.

DROP INDEX IF EXISTS idx_worker_tasks_project_worker;

ALTER TABLE worker_tasks DROP CONSTRAINT IF EXISTS worker_tasks_worker_project_fkey;

ALTER TABLE workers DROP CONSTRAINT IF EXISTS workers_pkey;
ALTER TABLE workers ADD CONSTRAINT workers_pkey PRIMARY KEY (id);

ALTER TABLE worker_tasks
    ADD CONSTRAINT worker_tasks_worker_id_fkey
    FOREIGN KEY (worker_id)
    REFERENCES workers(id)
    ON DELETE CASCADE;
