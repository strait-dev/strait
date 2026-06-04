-- Phase 2 of the (project_id, webhook_url) dedup: install the partial
-- unique index now that the previous migration collapsed any pre-existing duplicates.
--
-- Before this migration the create handler had no SQL-level dedup: a
-- flaky-network retry from a caller could land two rows pointing at the
-- same destination URL, and every fan-out event then dispatched twice.
-- The plan's idempotency-middleware fix could not ship because the
-- create response carries the webhook signing secret in plaintext
-- exactly once, so caching and replaying the response would re-expose
-- that secret on retry. The partial unique index closes the duplicate
-- hole at the SQL layer; the handler catches Postgres 23505 and returns
-- 409 Conflict with no secret material.
--
-- The partial WHERE active = TRUE matches the existing
-- idx_webhook_subscriptions_project_active scope and lets soft-deleted
-- (active=false) rows accumulate without participating in uniqueness.
--
-- CONCURRENTLY avoids the ACCESS EXCLUSIVE lock that would otherwise
-- block INSERT/UPDATE traffic on webhook_subscriptions during the index
-- build. golang-migrate runs each single-statement migration file
-- without a transaction wrapper, so CONCURRENTLY is accepted as long as
-- this is the only statement in the file (hence the split from the
-- previous backfill). IF NOT EXISTS keeps the migration idempotent on
-- retry.

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_webhook_subscriptions_url_unique_active
    ON webhook_subscriptions (project_id, webhook_url)
    WHERE active = TRUE;
