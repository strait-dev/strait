-- Per-project checkpoints for incremental audit chain verification.
--
-- VerifyAuditChain reads every event in a project's chain from epoch 0
-- through the tail and recomputes HMAC signatures in memory. For a
-- project with 10M rows that is expensive even under normal load, and
-- worse under the verify endpoint rate-limited at 1/min per project.
-- A successful verification never invalidates the prefix before the
-- last-verified event — if the chain was valid up to event X on scan
-- N, re-reading those same rows on scan N+1 cannot flip them to
-- invalid (signatures are deterministic under the per-epoch keys).
--
-- audit_chain_checkpoints stores the tail position of the last
-- successful verification per project so the incremental path only
-- has to read rows after that point. The full-chain path remains
-- available when incremental=false or when the checkpoint is missing,
-- and also serves as a periodic safety net (catches invalidations in
-- the already-verified prefix caused by a separate tamper vector).
--
-- Schema notes:
--   - project_id PRIMARY KEY bounds the table to one row per project.
--     A full_verify replaces the row in-place so the table size stays
--     O(num_projects) regardless of verification frequency.
--   - last_verified_event_id is the id of the tail event that verified
--     successfully on the last call. The next incremental verify starts
--     from the row immediately after this event.
--   - last_verified_at carries the wall-clock time of the last success
--     for observability — dashboards can alert when a project's
--     checkpoint is stale beyond the expected verification cadence.
--   - No FK to audit_events or projects: parity with audit_signing_keys
--     reasoning (compliance retention requires chain artifacts to
--     outlive project deletion).

CREATE TABLE IF NOT EXISTS audit_chain_checkpoints (
    project_id             TEXT        PRIMARY KEY,
    last_verified_event_id TEXT        NOT NULL,
    last_verified_at       TIMESTAMPTZ NOT NULL
);
