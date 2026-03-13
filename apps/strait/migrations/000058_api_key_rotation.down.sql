DROP INDEX IF EXISTS idx_api_keys_grace_expires_at;
ALTER TABLE api_keys
    DROP COLUMN IF EXISTS grace_expires_at,
    DROP COLUMN IF EXISTS replaced_by_key_id;
