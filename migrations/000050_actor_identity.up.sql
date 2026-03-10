-- Actor identity: lightweight cache of user info from external auth provider.
CREATE TABLE known_actors (
    id         TEXT PRIMARY KEY,
    email      TEXT,
    name       TEXT,
    avatar_url TEXT,
    synced_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Audit columns on core tables.
ALTER TABLE jobs ADD COLUMN created_by TEXT;
ALTER TABLE jobs ADD COLUMN updated_by TEXT;
ALTER TABLE workflows ADD COLUMN created_by TEXT;
ALTER TABLE workflows ADD COLUMN updated_by TEXT;
ALTER TABLE job_runs ADD COLUMN created_by TEXT;
ALTER TABLE workflow_runs ADD COLUMN created_by TEXT;
