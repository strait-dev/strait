-- Tighten audit_signing_keys against malformed ciphertext and track the
-- actor behind each rotation.
--
-- 1. octet_length(key_material) >= 28 rejects any row whose ciphertext is
--    shorter than an AES-GCM envelope can legally produce (12-byte nonce
--    + 16-byte authentication tag for an empty plaintext). The real
--    encrypt path always produces >= 44 bytes (nonce + sealed 32-byte
--    HMAC key), so 28 is a generous floor — a value that short could only
--    originate from a write path that skipped encryption entirely, which
--    this CHECK prevents from landing.
--
-- 2. created_by records the actor id that triggered the rotation. The
--    audit.key_rotated event already carries the actor; this mirrors it
--    at the key row so the forensic trail survives even if the chain
--    event for that rotation is somehow lost.
--
-- NOTE on the missing FOREIGN KEY: audit_signing_keys intentionally does
-- not reference projects(id). The parent audit_events table has never
-- had a project FK either, by design — audit rows must outlive tenant
-- deletion for compliance retention (SOC 2 7-year floor, HIPAA 6-year
-- floor). An ON DELETE CASCADE on the signing keys would silently
-- destroy chain verifiability after project deletion even when the
-- audit_events rows are still being retained, which an attacker could
-- abuse to mask tampering. A ON DELETE SET NULL or ON DELETE RESTRICT
-- path is feasible but is a larger policy decision (who gets to delete
-- a project with live audit history?) and belongs in a follow-up PR
-- alongside the projects.audit_retention column landing.

ALTER TABLE audit_signing_keys
    ADD CONSTRAINT audit_signing_keys_key_material_length
    CHECK (octet_length(key_material) >= 28);

ALTER TABLE audit_signing_keys
    ADD COLUMN IF NOT EXISTS created_by TEXT NULL;
