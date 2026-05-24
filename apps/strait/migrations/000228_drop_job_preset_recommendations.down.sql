-- Restore OOM-based preset recommendation table.
CREATE TABLE IF NOT EXISTS job_preset_recommendations (
    id                 TEXT PRIMARY KEY,
    job_id             TEXT NOT NULL UNIQUE,
    current_preset     TEXT NOT NULL,
    recommended_preset TEXT NOT NULL,
    oom_count          INT NOT NULL DEFAULT 0,
    window_start       TIMESTAMPTZ NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_job_preset_rec_expires ON job_preset_recommendations (expires_at);
