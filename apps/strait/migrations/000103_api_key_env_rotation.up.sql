ALTER TABLE api_keys
  ADD COLUMN environment_id TEXT,
  ADD COLUMN rotation_interval_days INT,
  ADD COLUMN next_rotation_at TIMESTAMPTZ,
  ADD COLUMN rotation_webhook_url TEXT;
