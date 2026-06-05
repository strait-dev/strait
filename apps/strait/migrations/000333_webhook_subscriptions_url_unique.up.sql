-- Phase 1 of the (project_id, webhook_url) dedup: collapse any duplicate
-- active rows that landed before the new unique constraint by deactivating
-- all but the most recently created row per (project_id, webhook_url).
--
-- The freshest row wins because callers who retried into a duplicate
-- typically discarded the older signing secret on the second response, so
-- the newer row matches the secret the caller is actually using. This is a
-- one-shot repair: once the next migration installs the partial unique index, the
-- store and handler refuse to let duplicates recur.
--
-- The CREATE UNIQUE INDEX CONCURRENTLY half is split into the next migration because
-- golang-migrate runs a multi-statement file inside a transaction, and
-- CONCURRENTLY cannot execute inside one. Keeping the backfill alone lets the
-- transactional wrap stay safe for the UPDATE while the index ships
-- non-transactionally in the next migration.

UPDATE webhook_subscriptions
SET active = FALSE
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY project_id, webhook_url
                   ORDER BY created_at DESC, id DESC
               ) AS rn
        FROM webhook_subscriptions
        WHERE active = TRUE
    ) ranked
    WHERE ranked.rn > 1
);
