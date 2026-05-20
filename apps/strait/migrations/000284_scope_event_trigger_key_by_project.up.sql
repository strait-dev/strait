ALTER TABLE event_triggers
	DROP CONSTRAINT IF EXISTS event_triggers_event_key_key;

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_event_triggers_project_event_key_unique
	ON event_triggers(project_id, event_key);
