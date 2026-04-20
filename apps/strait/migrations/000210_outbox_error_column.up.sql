-- R4 Phase 8: add an error column to enqueue_outbox so the flusher can
-- dead-letter entries that fail enqueue (e.g. missing job) instead of
-- retrying them forever.

ALTER TABLE enqueue_outbox ADD COLUMN IF NOT EXISTS error TEXT;
UPDATE schema_version SET version = 210, updated_at = NOW();
