DROP INDEX IF EXISTS idx_api_keys_org_id;
ALTER TABLE api_keys DROP COLUMN IF EXISTS org_id;
