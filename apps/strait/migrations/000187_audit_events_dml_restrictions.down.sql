DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'strait_app') THEN
        GRANT UPDATE, DELETE ON audit_events TO strait_app;
    END IF;
END $$;
