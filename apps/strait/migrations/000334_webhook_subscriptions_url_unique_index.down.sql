-- Roll back the partial unique index. The previous backfill that
-- deactivated pre-existing duplicate rows is intentionally not reversed:
-- the freshest row was kept active, and reactivating the deactivated
-- copies would re-create the duplicate-dispatch surface this pair of
-- migrations closed.

DROP INDEX CONCURRENTLY IF EXISTS idx_webhook_subscriptions_url_unique_active;
