-- Per-project cap on the number of rows a single audit export can stream.
--
-- 0 means "use the server default" (AUDIT_EXPORT_ROW_CAP_DEFAULT env var,
-- default 1,000,000). A positive value overrides the default for this
-- project. The export handler (handleExportAuditEvents) emits an
-- audit.export_capped event and a trailing _capped sentinel row when a
-- stream terminates early because the cap was reached.

ALTER TABLE project_quotas
    ADD COLUMN IF NOT EXISTS audit_export_row_cap BIGINT NOT NULL DEFAULT 0;
