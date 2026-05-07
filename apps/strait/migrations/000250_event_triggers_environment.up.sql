-- Add environment scope to event_triggers. Nullable so legacy rows
-- (created before this column existed) remain accessible to project-wide
-- callers. New triggers populate environment_id from the run/job's
-- environment when applicable.
ALTER TABLE event_triggers
  ADD COLUMN environment_id TEXT;

-- Partial index for env-scoped lookups by status/expiry. Mirrors the
-- existing idx_event_triggers_project pattern but adds the env axis.
CREATE INDEX idx_event_triggers_project_env_status
    ON event_triggers(project_id, environment_id, status);
