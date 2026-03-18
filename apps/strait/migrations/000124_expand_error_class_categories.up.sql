CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_error_class_timeout ON job_runs(error_class) WHERE error_class = 'timeout';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_error_class_oom ON job_runs(error_class) WHERE error_class = 'oom';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_error_class_connection ON job_runs(error_class) WHERE error_class = 'connection';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_error_class_budget ON job_runs(error_class) WHERE error_class = 'budget';
