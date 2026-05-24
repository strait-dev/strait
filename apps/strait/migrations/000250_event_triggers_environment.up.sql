-- Add environment scope to event_triggers. Nullable so legacy rows
-- (created before this column existed) remain accessible to project-wide
-- callers. New triggers populate environment_id from the run/job's
-- environment when applicable.
ALTER TABLE event_triggers
  ADD COLUMN environment_id TEXT;
