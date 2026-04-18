-- Audit retention: per-project retention window in days.
--
-- 0 means "use the server default" (AUDIT_RETENTION_DEFAULT_DAYS env var).
-- A positive value overrides the default for this project.
--
-- The scheduler reaper task deletes audit_events where
-- created_at < NOW() - interval '<retention> days'. Only TAIL deletions
-- (oldest-first) are allowed, which means VerifyAuditChain must accept a
-- chain whose earliest row has a non-ZeroHash previous_hash — the
-- verifier takes that hash as the chain start anchor.

ALTER TABLE project_quotas
    ADD COLUMN IF NOT EXISTS audit_retention_days INT NOT NULL DEFAULT 0;
