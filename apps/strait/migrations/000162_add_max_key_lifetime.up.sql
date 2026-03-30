ALTER TABLE project_quotas
    ADD COLUMN IF NOT EXISTS max_key_lifetime_days INT NOT NULL DEFAULT 0;
