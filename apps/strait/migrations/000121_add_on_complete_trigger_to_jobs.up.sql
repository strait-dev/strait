ALTER TABLE jobs ADD COLUMN on_complete_trigger_workflow TEXT;
ALTER TABLE jobs ADD COLUMN on_complete_payload_mapping JSONB;
