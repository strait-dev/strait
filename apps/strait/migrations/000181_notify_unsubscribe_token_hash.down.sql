DROP INDEX IF EXISTS idx_notify_unsubscribe_token_hash;

UPDATE unsubscribe_tokens
SET token = token_hash
WHERE token IS NULL OR token = '';

ALTER TABLE unsubscribe_tokens
    ALTER COLUMN token SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'unsubscribe_tokens_token_key'
    ) THEN
        ALTER TABLE unsubscribe_tokens
            ADD CONSTRAINT unsubscribe_tokens_token_key UNIQUE (token);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_notify_unsubscribe_token
    ON unsubscribe_tokens(token)
    WHERE used_at IS NULL;

ALTER TABLE unsubscribe_tokens
    DROP COLUMN IF EXISTS token_hash;
