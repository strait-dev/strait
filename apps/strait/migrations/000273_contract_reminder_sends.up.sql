CREATE TABLE IF NOT EXISTS contract_reminder_sends (
    org_id TEXT NOT NULL,
    contract_end_date DATE NOT NULL,
    reminder_window_days INT NOT NULL,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (org_id, contract_end_date, reminder_window_days)
);
