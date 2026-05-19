-- Scope worker identity by project so tenant-local worker IDs cannot collide.

ALTER TABLE worker_tasks DROP CONSTRAINT IF EXISTS worker_tasks_worker_id_fkey;

ALTER TABLE workers DROP CONSTRAINT IF EXISTS workers_pkey;
ALTER TABLE workers ADD CONSTRAINT workers_pkey PRIMARY KEY (project_id, id);

ALTER TABLE worker_tasks
    ADD CONSTRAINT worker_tasks_worker_project_fkey
    FOREIGN KEY (project_id, worker_id)
    REFERENCES workers(project_id, id)
    ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_worker_tasks_project_worker
    ON worker_tasks (project_id, worker_id, status);
