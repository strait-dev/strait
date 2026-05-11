ALTER TABLE api_keys
    ADD COLUMN rotation_webhook_secret bytea;

COMMENT ON COLUMN api_keys.rotation_webhook_secret IS
    'HMAC signing secret for rotation_webhook_url notifications. Encrypted at rest via crypto.Encryptor.';
