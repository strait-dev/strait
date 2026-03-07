ALTER TABLE jobs ADD COLUMN environment_id TEXT REFERENCES environments(id);
ALTER TABLE job_versions ADD COLUMN environment_id TEXT;
