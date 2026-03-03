CREATE TABLE webhook_deliveries (
    id               TEXT        PRIMARY KEY,
    run_id           TEXT        NOT NULL,
    job_id           TEXT        NOT NULL,
    webhook_url      TEXT        NOT NULL,
    status           TEXT        NOT NULL DEFAULT 'pending',
    attempts         INT         NOT NULL DEFAULT 0,
    max_attempts     INT         NOT NULL DEFAULT 3,
    last_status_code INT,
    last_error       TEXT,
    next_retry_at    TIMESTAMPTZ,
    delivered_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_webhook_deliveries_run_id ON webhook_deliveries(run_id);
CREATE INDEX idx_webhook_deliveries_status ON webhook_deliveries(status) WHERE status IN ('pending', 'failed');
