ALTER TABLE sent_usage_reports
    ADD COLUMN IF NOT EXISTS send_status TEXT NOT NULL DEFAULT 'sent';

ALTER TABLE sent_usage_reports
    ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ;

ALTER TABLE sent_usage_reports
    ALTER COLUMN sent_at DROP NOT NULL;

UPDATE sent_usage_reports
SET send_status = 'sent',
    claimed_at = NULL
WHERE send_status IS NULL OR send_status = '';

ALTER TABLE sent_usage_reports
    ADD CONSTRAINT sent_usage_reports_send_status_check
    CHECK (send_status IN ('claimed', 'sent'));

CREATE INDEX IF NOT EXISTS idx_sent_usage_reports_claimed
    ON sent_usage_reports(period_end, claimed_at)
    WHERE send_status = 'claimed';
