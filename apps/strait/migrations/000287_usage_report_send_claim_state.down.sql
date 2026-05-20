DROP INDEX IF EXISTS idx_sent_usage_reports_claimed;

ALTER TABLE sent_usage_reports
    DROP CONSTRAINT IF EXISTS sent_usage_reports_send_status_check;

UPDATE sent_usage_reports
SET sent_at = COALESCE(sent_at, claimed_at, NOW());

ALTER TABLE sent_usage_reports
    ALTER COLUMN sent_at SET NOT NULL;

ALTER TABLE sent_usage_reports
    DROP COLUMN IF EXISTS claimed_at;

ALTER TABLE sent_usage_reports
    DROP COLUMN IF EXISTS send_status;
