ALTER TABLE project_quotas
    ADD COLUMN IF NOT EXISTS audit_retention_override_set BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE project_quotas
SET audit_retention_override_set = TRUE
WHERE audit_retention_days > 0;
