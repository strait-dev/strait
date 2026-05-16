ALTER TABLE project_quotas
    ADD COLUMN IF NOT EXISTS audit_retention_override_set BOOLEAN;

UPDATE project_quotas
SET audit_retention_override_set = TRUE
WHERE audit_retention_days > 0;

UPDATE project_quotas
SET audit_retention_override_set = FALSE
WHERE audit_retention_override_set IS NULL;

ALTER TABLE project_quotas
    ALTER COLUMN audit_retention_override_set SET DEFAULT FALSE;
