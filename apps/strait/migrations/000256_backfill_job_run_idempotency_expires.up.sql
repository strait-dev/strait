-- Wave 2 Phase 3: backfill expires_at on legacy job_run_idempotency rows.
--
-- Migration 000065 created job_run_idempotency for cross-partition dedup
-- and seeded it from job_runs with `created_at > NOW() - INTERVAL '30 days'`
-- as a one-time backfill. The seed did not populate expires_at, so those
-- rows are permanently retained by any GC keyed on expires_at < NOW().
--
-- The read window in GetRunByIdempotencyKey is 24 hours past finished_at;
-- the legacy seed is much older than that, so all seeded rows are well
-- outside any replay window. Set expires_at = created_at + INTERVAL '24
-- hours' on NULL rows so the GC introduced in the same change can drain
-- them in bounded batches.
--
-- The UPDATE is a single bulk statement; the table is small (only rows
-- carrying an idempotency key) and rarely updated, so the lock contention
-- is negligible.

UPDATE job_run_idempotency
SET expires_at = created_at + INTERVAL '24 hours'
WHERE expires_at IS NULL;
