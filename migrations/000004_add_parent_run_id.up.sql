ALTER TABLE job_runs ADD COLUMN parent_run_id TEXT REFERENCES job_runs(id);
CREATE INDEX idx_job_runs_parent_run_id ON job_runs(parent_run_id) WHERE parent_run_id IS NOT NULL;
