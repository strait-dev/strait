CREATE TABLE IF NOT EXISTS job_groups (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, slug)
);

ALTER TABLE jobs ADD COLUMN IF NOT EXISTS group_id TEXT REFERENCES job_groups(id);
CREATE INDEX IF NOT EXISTS idx_jobs_group_id ON jobs(group_id) WHERE group_id IS NOT NULL;
