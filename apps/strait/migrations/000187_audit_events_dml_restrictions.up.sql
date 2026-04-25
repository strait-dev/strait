-- Restrict the application role to INSERT + SELECT only on audit_events.
-- UPDATE is allowed only for the signature column (set during chain computation).
-- DELETE is reserved for the superuser retention reaper.
--
-- This prevents a compromised application process from rewriting or
-- deleting audit events, making the HMAC chain tamper-evident even
-- against application-level compromise.

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'strait_app') THEN
        REVOKE UPDATE, DELETE ON audit_events FROM strait_app;
        GRANT INSERT, SELECT ON audit_events TO strait_app;
        GRANT UPDATE (signature) ON audit_events TO strait_app;
    END IF;
END $$;
