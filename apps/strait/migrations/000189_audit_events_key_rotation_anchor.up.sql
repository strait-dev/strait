-- Key rotation forensic anchor columns for audit events.
--
-- Rotating the HMAC signing secret would otherwise invalidate every
-- pre-rotation signature. Anchor rows are positive forensic markers that
-- record the rotation event itself and serve as chain boundaries in
-- VerifyAuditChain: when an anchor is encountered, the expected
-- previous_hash is reset to the anchor's own signature before continuing.
--
-- Both columns default to FALSE / 0 so existing rows are unaffected. The
-- canonical HMAC form (schema v2) is NOT extended — anchor status is an
-- out-of-band marker that only influences the verifier's chain-boundary
-- logic, not the per-row signature.
--
-- Index (project_id, rotation_epoch, created_at DESC) supports efficient
-- per-epoch scans during chain verification.

ALTER TABLE audit_events
    ADD COLUMN IF NOT EXISTS is_anchor       BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS rotation_epoch  INTEGER NOT NULL DEFAULT 0;

ALTER TABLE audit_events_deadletter
    ADD COLUMN IF NOT EXISTS is_anchor       BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS rotation_epoch  INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_audit_events_project_epoch_created
    ON audit_events(project_id, rotation_epoch, created_at DESC);
