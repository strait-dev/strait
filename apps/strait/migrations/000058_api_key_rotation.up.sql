ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS replaced_by_key_id TEXT,
    ADD COLUMN IF NOT EXISTS grace_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_api_keys_grace_expires_at ON api_keys(grace_expires_at);
