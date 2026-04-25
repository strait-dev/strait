-- safety-ok: PG 11+ stores immutable default in pg_attribute.attmissingval; no table rewrite
ALTER TABLE agents ADD COLUMN enabled BOOLEAN NOT NULL DEFAULT true;
