CREATE TABLE job_secrets (
    id                TEXT        PRIMARY KEY,
    project_id        TEXT        NOT NULL,
    job_id            TEXT        REFERENCES jobs(id) ON DELETE CASCADE,
    environment       TEXT        NOT NULL DEFAULT 'dev',
    secret_key        TEXT        NOT NULL,
    encrypted_value   TEXT        NOT NULL,
    key_version       INT         NOT NULL DEFAULT 1,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, job_id, environment, secret_key)
);

CREATE INDEX idx_job_secrets_project_env ON job_secrets(project_id, environment);
