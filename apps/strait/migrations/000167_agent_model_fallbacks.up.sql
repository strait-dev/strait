ALTER TABLE agents ADD COLUMN IF NOT EXISTS model_fallbacks TEXT[] DEFAULT '{}';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS provider_secrets_encrypted TEXT DEFAULT '';
