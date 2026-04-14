-- Keep the binary/schema compatibility check aligned with the retry
-- lineage index migration, which must run separately because it uses
-- CREATE INDEX CONCURRENTLY.
UPDATE schema_version SET version = 205, updated_at = NOW();
