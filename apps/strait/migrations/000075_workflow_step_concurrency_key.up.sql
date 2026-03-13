ALTER TABLE workflow_steps
    ADD COLUMN concurrency_key TEXT NOT NULL DEFAULT '';

ALTER TABLE workflow_version_steps
    ADD COLUMN concurrency_key TEXT NOT NULL DEFAULT '';
