CREATE INDEX IF NOT EXISTS idx_job_runs_machine_id
  ON job_runs (machine_id)
  WHERE machine_id IS NOT NULL AND execution_mode = 'managed';
