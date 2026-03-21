ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS org_id TEXT;
CREATE INDEX IF NOT EXISTS idx_api_keys_org_id ON api_keys(org_id) WHERE org_id IS NOT NULL;
