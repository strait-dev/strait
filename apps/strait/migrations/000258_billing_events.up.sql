-- Persistent idempotency log for inbound Stripe webhook events. Replaces the
-- in-memory replayCache (sync.Map) used by billing/webhook.go so that
-- duplicate events delivered after a process restart, leader change, or
-- multi-instance fan-in are still rejected.
--
-- Each row is keyed by Stripe's event id, which is globally unique and stable
-- across redeliveries, so a primary key on event_id is the natural dedupe
-- mechanism. The handler INSERTs ON CONFLICT DO NOTHING up front; if the row
-- already exists, the event has been seen and is short-circuited.

CREATE TABLE IF NOT EXISTS billing_events (
    event_id     TEXT PRIMARY KEY,
    event_type   TEXT NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ,
    result       TEXT,
    error        TEXT
);

CREATE INDEX IF NOT EXISTS idx_billing_events_received_at
    ON billing_events (received_at);
