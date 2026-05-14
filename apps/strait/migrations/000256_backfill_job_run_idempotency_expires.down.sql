-- Reverse Wave 2 Phase 3 backfill: clear expires_at to restore prior NULL state.
-- Safe because no code reads expires_at outside the GC introduced in the
-- same change.
UPDATE job_run_idempotency
SET expires_at = NULL
WHERE expires_at IS NOT NULL;
