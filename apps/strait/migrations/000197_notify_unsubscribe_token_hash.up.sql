ALTER TABLE unsubscribe_tokens
    ADD COLUMN IF NOT EXISTS token_hash TEXT;

UPDATE unsubscribe_tokens
SET token_hash = encode(digest(token, 'sha256'), 'hex')
WHERE token_hash IS NULL
  AND token IS NOT NULL;

-- safety-ok: notify feature is pre-launch; unsubscribe_tokens has no production rows yet
ALTER TABLE unsubscribe_tokens
    ALTER COLUMN token_hash SET NOT NULL;

ALTER TABLE unsubscribe_tokens
    ALTER COLUMN token DROP NOT NULL;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'unsubscribe_tokens_token_key'
    ) THEN
        ALTER TABLE unsubscribe_tokens DROP CONSTRAINT unsubscribe_tokens_token_key;
    END IF;
END $$;

DROP INDEX IF EXISTS idx_notify_unsubscribe_token;

-- safety-ok: notify feature is pre-launch; unsubscribe_tokens has no production rows yet
CREATE UNIQUE INDEX IF NOT EXISTS idx_notify_unsubscribe_token_hash
    ON unsubscribe_tokens(token_hash)
    WHERE used_at IS NULL;
