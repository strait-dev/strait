CREATE INDEX idx_job_runs_dead_letter ON job_runs(project_id) WHERE status = 'dead_letter';
