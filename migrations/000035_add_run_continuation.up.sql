ALTER TABLE job_runs ADD COLUMN continuation_of TEXT NULL REFERENCES job_runs(id);
ALTER TABLE job_runs ADD COLUMN lineage_depth INT NOT NULL DEFAULT 0;
CREATE INDEX idx_job_runs_continuation_of ON job_runs(continuation_of) WHERE continuation_of IS NOT NULL;
