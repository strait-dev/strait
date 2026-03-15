ALTER TABLE jobs ADD COLUMN IF NOT EXISTS queue_depth_alert_threshold INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS queue_depth_alert_threshold INT;
