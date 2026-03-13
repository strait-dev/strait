ALTER TABLE workflow_steps
    ADD COLUMN resource_class TEXT NOT NULL DEFAULT 'small';

ALTER TABLE workflow_version_steps
    ADD COLUMN resource_class TEXT NOT NULL DEFAULT 'small';
