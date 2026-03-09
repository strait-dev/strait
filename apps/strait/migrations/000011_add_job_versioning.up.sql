ALTER TABLE jobs ADD COLUMN version INT NOT NULL DEFAULT 1;

ALTER TABLE job_runs ADD COLUMN job_version INT NOT NULL DEFAULT 1;

CREATE TABLE job_versions (
    id             TEXT        PRIMARY KEY,
    job_id         TEXT        NOT NULL REFERENCES jobs(id),
    version        INT         NOT NULL,
    name           TEXT        NOT NULL,
    slug           TEXT        NOT NULL,
    description    TEXT,
    cron           TEXT,
    payload_schema JSONB,
    endpoint_url   TEXT        NOT NULL,
    max_attempts   INT         NOT NULL,
    timeout_secs   INT         NOT NULL,
    webhook_url    TEXT,
    webhook_secret TEXT,
    run_ttl_secs   INT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (job_id, version)
);

CREATE INDEX idx_job_versions_job_id ON job_versions(job_id);
