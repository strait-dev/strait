ALTER TABLE jobs DROP COLUMN IF EXISTS queue_depth_alert_threshold;
ALTER TABLE job_versions DROP COLUMN IF EXISTS queue_depth_alert_threshold;
