DROP TRIGGER IF EXISTS trg_outbox_claim_sync_on_insert ON enqueue_outbox;
DROP FUNCTION IF EXISTS outbox_claim_sync_on_insert();
DROP INDEX IF EXISTS idx_outbox_claims_ready_created;
