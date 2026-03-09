-- Prefix pattern index for wildcard event key matching (LIKE 'prefix%')
CREATE INDEX idx_event_triggers_event_key_prefix ON event_triggers(event_key text_pattern_ops);
