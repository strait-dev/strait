DROP INDEX IF EXISTS idx_event_triggers_project_env_status;
ALTER TABLE event_triggers DROP COLUMN IF EXISTS environment_id;
