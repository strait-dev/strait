-- Restore code_deployments table and the columns it previously owned.
CREATE TABLE IF NOT EXISTS code_deployments (
    id                  TEXT PRIMARY KEY,
    job_id              TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    project_id          TEXT NOT NULL,
    version             INT NOT NULL DEFAULT 1,
    status              TEXT NOT NULL DEFAULT 'pending',
    runtime             TEXT NOT NULL,
    source_hash         TEXT NOT NULL,
    source_size_bytes   BIGINT NOT NULL DEFAULT 0,
    source_uri          TEXT NOT NULL,
    built_image_uri     TEXT,
    built_image_digest  TEXT,
    build_logs          TEXT,
    error_message       TEXT,
    created_by          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at         TIMESTAMPTZ
);

ALTER TABLE jobs ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT 'image';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS runtime TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS active_deployment_id TEXT REFERENCES code_deployments(id) ON DELETE SET NULL;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS rollback_source_deployment_id TEXT REFERENCES code_deployments(id) ON DELETE SET NULL;

ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS deployment_id TEXT REFERENCES code_deployments(id) ON DELETE SET NULL;
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS pinned_image_uri TEXT;
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS pinned_image_digest TEXT;
