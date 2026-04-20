-- Forensic enrichment columns for audit events.
--
-- These are additive and default to empty so existing rows continue to
-- verify under the old canonical signature form. New events (schema_version
-- >= 2) include the forensic fields in their HMAC canonical form, so the
-- verifier must branch on schema_version when computing the expected
-- signature.
--
-- remote_ip / user_agent / request_id / trace_id are forensic metadata
-- populated from the request middleware. schema_version is the integer
-- version of the audit event contract — bumped whenever a canonical form
-- change lands.

ALTER TABLE audit_events
    ADD COLUMN IF NOT EXISTS remote_ip      TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS user_agent     TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS request_id     TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trace_id       TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS schema_version SMALLINT NOT NULL DEFAULT 1;

ALTER TABLE audit_events_deadletter
    ADD COLUMN IF NOT EXISTS remote_ip      TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS user_agent     TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS request_id     TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS trace_id       TEXT    NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS schema_version SMALLINT NOT NULL DEFAULT 1;

CREATE INDEX IF NOT EXISTS idx_audit_events_request_id
    ON audit_events(request_id) WHERE request_id != '';
