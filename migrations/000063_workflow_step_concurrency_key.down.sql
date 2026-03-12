ALTER TABLE workflow_version_steps
    DROP COLUMN IF EXISTS concurrency_key;

ALTER TABLE workflow_steps
    DROP COLUMN IF EXISTS concurrency_key;
