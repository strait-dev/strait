-- safety-ok: outbox_claims is a narrow queue-side claim table; golang-migrate runs this migration in a transaction, so CONCURRENTLY cannot be used here.
CREATE INDEX IF NOT EXISTS idx_outbox_claims_ready_batch
    ON outbox_claims(batch_id, outbox_id)
    WHERE status = 'ready';
