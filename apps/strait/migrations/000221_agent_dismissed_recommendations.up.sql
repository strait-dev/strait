-- safety-ok: PG 11+ stores immutable default in pg_attribute.attmissingval; no table rewrite
ALTER TABLE agents ADD COLUMN dismissed_recommendations JSONB NOT NULL DEFAULT '[]'::jsonb;
