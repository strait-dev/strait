-- 000252: Persist response headers alongside the cached body so replays
-- reproduce the original Content-Type, Location, Set-Cookie, ETag, etc.
-- Pre-fix replays unconditionally emitted Content-Type: application/json
-- and dropped every other handler-set header.

ALTER TABLE idempotency_keys
  ADD COLUMN IF NOT EXISTS response_headers JSONB;
