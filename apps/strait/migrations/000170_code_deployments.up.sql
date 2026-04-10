-- code_deployments tracks each code-first deployment of a job.
-- Each deployment corresponds to one tarball upload → BuildKit build → image push cycle.
CREATE TABLE IF NOT EXISTS code_deployments (
    id                  TEXT PRIMARY KEY,
    job_id              TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    project_id          TEXT NOT NULL,
    version             INT NOT NULL DEFAULT 1,
    status              TEXT NOT NULL DEFAULT 'pending',   -- pending, building, ready, failed
    runtime             TEXT NOT NULL,                     -- python, typescript, ruby, rust, go
    source_hash         TEXT NOT NULL,                     -- SHA-256 of the uploaded tarball
    source_size_bytes   BIGINT NOT NULL DEFAULT 0,
    source_uri          TEXT NOT NULL,                     -- object store key (e.g. projects/{project_id}/jobs/{job_id}/deploys/{id}.tar.gz)
    built_image_uri     TEXT,                              -- full image URI after a successful build
    built_image_digest  TEXT,                              -- image digest (sha256:...) for pinning
    build_logs          TEXT,                              -- captured stdout/stderr from BuildKit
    error_message       TEXT,
    created_by          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_code_deployments_job_id
    ON code_deployments (job_id);

CREATE INDEX IF NOT EXISTS idx_code_deployments_project_id
    ON code_deployments (project_id);

-- Partial index for fast polling of in-progress builds.
CREATE INDEX IF NOT EXISTS idx_code_deployments_active
    ON code_deployments (status, job_id)
    WHERE status IN ('pending', 'building');

-- jobs: source_type controls whether the job uses a user-supplied image or code-first deploys.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS source_type          TEXT NOT NULL DEFAULT 'image';
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS runtime              TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS active_deployment_id TEXT REFERENCES code_deployments(id) ON DELETE SET NULL;

-- job_runs: pin the exact image at queue time so retries always use the same version.
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS deployment_id      TEXT REFERENCES code_deployments(id) ON DELETE SET NULL;
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS pinned_image_uri    TEXT;
ALTER TABLE job_runs ADD COLUMN IF NOT EXISTS pinned_image_digest TEXT;
