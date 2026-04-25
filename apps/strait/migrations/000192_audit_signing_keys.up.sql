-- Per-epoch audit signing keys.
--
-- Prior to this migration VerifyAuditChain verified every event — across
-- all rotation epochs — under a single global HMAC key derived from
-- INTERNAL_SECRET. That made rotation only a forensic marker, not a real
-- key change: compromise of the global key retroactively compromised
-- every past event's signature.
--
-- This table stores a distinct HMAC key per (project, rotation_epoch).
-- key_material is AES-GCM encrypted at rest using the same
-- HKDF(INTERNAL_SECRET, "secret-store-encryption") envelope used by
-- job_secrets. RotateAuditSigningKey derives, encrypts, and inserts a
-- fresh key under the new epoch before emitting the anchor row so the
-- anchor itself is signed under the new epoch's key. VerifyAuditChain
-- groups events by rotation_epoch and verifies each group under the
-- corresponding key.
--
-- Epoch 0 has no row: pre-rotation chains fall back to the configured
-- global auditSigningKey for backwards compatibility with installations
-- that existed before per-epoch keys landed.

CREATE TABLE IF NOT EXISTS audit_signing_keys (
    project_id       TEXT NOT NULL,
    rotation_epoch   INTEGER NOT NULL,
    key_material     BYTEA NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, rotation_epoch)
);
