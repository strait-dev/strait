CREATE TABLE IF NOT EXISTS dedup_log (
    project_id TEXT NOT NULL,
    dedup_key  TEXT NOT NULL,
    count      INT NOT NULL DEFAULT 1,
    first_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (project_id, dedup_key)
);

CREATE INDEX IF NOT EXISTS idx_notify_dedup_log_expiry
    ON dedup_log(expires_at);
