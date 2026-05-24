CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_outbox_claims_ready_created
    ON outbox_claims(created_at ASC, outbox_id ASC)
    WHERE status = 'ready';

CREATE OR REPLACE FUNCTION outbox_claim_sync_on_insert() RETURNS trigger AS $$
BEGIN
  IF NEW.consumed_at IS NULL THEN
    INSERT INTO outbox_claims (outbox_id, status, created_at, updated_at)
    VALUES (NEW.id, 'ready', COALESCE(NEW.created_at, NOW()), NOW())
    ON CONFLICT (outbox_id) DO NOTHING;
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_outbox_claim_sync_on_insert ON enqueue_outbox;

CREATE TRIGGER trg_outbox_claim_sync_on_insert
AFTER INSERT ON enqueue_outbox
FOR EACH ROW
EXECUTE FUNCTION outbox_claim_sync_on_insert();

INSERT INTO outbox_claims (outbox_id, status, created_at, updated_at)
SELECT id, 'ready', created_at, NOW()
FROM enqueue_outbox
WHERE consumed_at IS NULL
ON CONFLICT (outbox_id) DO NOTHING;
