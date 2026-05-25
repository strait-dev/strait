-- safety-ok: outbox_claims is a narrow queue-side claim table; golang-migrate runs this migration in a transaction, so CONCURRENTLY cannot be used here.
CREATE INDEX IF NOT EXISTS idx_outbox_claims_acked_updated
    ON outbox_claims(updated_at ASC, outbox_id ASC)
    WHERE status = 'acked';
