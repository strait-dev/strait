ALTER TABLE jobs ADD COLUMN IF NOT EXISTS dlq_alert_threshold INT;
ALTER TABLE job_versions ADD COLUMN IF NOT EXISTS dlq_alert_threshold INT;
