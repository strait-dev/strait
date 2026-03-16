ALTER TABLE jobs DROP COLUMN IF EXISTS dlq_alert_threshold;
ALTER TABLE job_versions DROP COLUMN IF EXISTS dlq_alert_threshold;
