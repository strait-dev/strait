ALTER TABLE deployment_versions
    ADD COLUMN IF NOT EXISTS strategy TEXT NOT NULL DEFAULT 'direct',
    ADD COLUMN IF NOT EXISTS canary_percent INTEGER,
    ADD COLUMN IF NOT EXISTS canary_duration INTERVAL;
