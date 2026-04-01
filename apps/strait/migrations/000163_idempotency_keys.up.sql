-- 000163: General-purpose idempotency keys table.
-- Supports the insert-pending-first pattern: a pending row is inserted before
-- the handler executes, then updated with the cached response on completion.
-- Keys are scoped per-project and expire after a configurable TTL (default 24h).

CREATE TABLE IF NOT EXISTS idempotency_keys (
  project_id      TEXT        NOT NULL,
  key             TEXT        NOT NULL,
  status          TEXT        NOT NULL DEFAULT 'pending',
  response_status INT,
  response_body   JSONB,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at      TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '24 hours',
  PRIMARY KEY (project_id, key)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_keys_expires
  ON idempotency_keys (expires_at)
  WHERE expires_at IS NOT NULL;
