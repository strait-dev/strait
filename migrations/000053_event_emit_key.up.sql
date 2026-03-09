-- Event chaining: auto-emit event on step completion
ALTER TABLE workflow_steps ADD COLUMN event_emit_key TEXT;
ALTER TABLE workflow_version_steps ADD COLUMN event_emit_key TEXT;
