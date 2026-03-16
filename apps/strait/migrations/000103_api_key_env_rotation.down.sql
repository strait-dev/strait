ALTER TABLE api_keys
  DROP COLUMN IF EXISTS environment_id,
  DROP COLUMN IF EXISTS rotation_interval_days,
  DROP COLUMN IF EXISTS next_rotation_at,
  DROP COLUMN IF EXISTS rotation_webhook_url;
