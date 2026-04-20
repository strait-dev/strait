-- R4 Phase 5: schema versioning.
-- The binary embeds an expected schema version; startup compares against
-- this table and refuses to boot on mismatch. Each future migration
-- appends an UPDATE to bump the version.

CREATE TABLE IF NOT EXISTS schema_version (
    id         INT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    version    INT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO schema_version (version) VALUES (209)
ON CONFLICT (id) DO UPDATE SET version = 196, updated_at = NOW();
