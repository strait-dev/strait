CREATE TABLE job_memory (
    id             TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    job_id         TEXT NOT NULL,
    project_id     TEXT NOT NULL,
    memory_key     TEXT NOT NULL,
    value          JSONB NOT NULL,
    size_bytes     INT NOT NULL DEFAULT 0,
    ttl_expires_at TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (job_id, memory_key)
);
CREATE INDEX idx_job_memory_job ON job_memory(job_id);
CREATE INDEX idx_job_memory_ttl ON job_memory(ttl_expires_at) WHERE ttl_expires_at IS NOT NULL;

ALTER TABLE project_quotas ADD COLUMN max_memory_per_key_bytes INT DEFAULT 1048576;
ALTER TABLE project_quotas ADD COLUMN max_memory_per_job_bytes INT DEFAULT 10485760;
