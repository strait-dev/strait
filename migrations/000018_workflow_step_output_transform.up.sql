-- Add output_transform column to workflow_steps and workflow_version_steps
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS output_transform TEXT NOT NULL DEFAULT '';
ALTER TABLE workflow_version_steps ADD COLUMN IF NOT EXISTS output_transform TEXT NOT NULL DEFAULT '';
