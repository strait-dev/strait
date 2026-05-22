DROP INDEX IF EXISTS idx_event_triggers_project_event_key_unique;

ALTER TABLE event_triggers
	ADD CONSTRAINT event_triggers_event_key_key UNIQUE (event_key);
