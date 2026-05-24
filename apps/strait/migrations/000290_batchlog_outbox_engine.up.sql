CREATE TABLE IF NOT EXISTS outbox_batches (
    id BIGSERIAL PRIMARY KEY,
    sealed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS outbox_claims (
    outbox_id TEXT PRIMARY KEY,
    batch_id BIGINT REFERENCES outbox_batches(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'ready' CHECK (status IN ('ready', 'leased', 'acked')),
    lease_owner TEXT,
    lease_expires_at TIMESTAMPTZ,
    claimed_at TIMESTAMPTZ,
    attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbox_claims_ready_batch
    ON outbox_claims(batch_id, outbox_id)
    WHERE status = 'ready';

CREATE INDEX IF NOT EXISTS idx_outbox_claims_lease_expiry
    ON outbox_claims(lease_expires_at)
    WHERE status = 'leased';
