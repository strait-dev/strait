ALTER TABLE workflow_version_steps
    DROP COLUMN IF EXISTS resource_class;

ALTER TABLE workflow_steps
    DROP COLUMN IF EXISTS resource_class;
