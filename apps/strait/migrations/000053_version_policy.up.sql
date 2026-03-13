-- Smart version detection: control how queued runs handle new deployments.
ALTER TABLE jobs ADD COLUMN version_policy TEXT NOT NULL DEFAULT 'pin';
ALTER TABLE workflows ADD COLUMN version_policy TEXT NOT NULL DEFAULT 'pin';

-- Backwards compatibility flag for versions.
ALTER TABLE job_versions ADD COLUMN backwards_compatible BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE workflow_versions ADD COLUMN backwards_compatible BOOLEAN NOT NULL DEFAULT FALSE;
