-- Audit: track who sent the event that resolved a trigger.
ALTER TABLE event_triggers ADD COLUMN sent_by TEXT NOT NULL DEFAULT '';
