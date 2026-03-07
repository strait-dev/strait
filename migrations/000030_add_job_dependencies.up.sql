CREATE TABLE IF NOT EXISTS job_dependencies (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    depends_on_job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    condition TEXT NOT NULL DEFAULT 'completed',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(job_id, depends_on_job_id)
);

CREATE INDEX IF NOT EXISTS idx_job_dependencies_job_id ON job_dependencies(job_id);
