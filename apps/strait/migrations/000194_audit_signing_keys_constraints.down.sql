ALTER TABLE audit_signing_keys
    DROP CONSTRAINT IF EXISTS audit_signing_keys_key_material_length;

ALTER TABLE audit_signing_keys
    DROP COLUMN IF EXISTS created_by;
