-- Outbox history table for consumed and quarantined outbox rows.
-- Partitioned by consumed_at to enable efficient month-level drops.

CREATE TABLE IF NOT EXISTS enqueue_outbox_history (
    id                TEXT NOT NULL,
    project_id        TEXT NOT NULL,
    job_id            TEXT NOT NULL,
    payload           JSONB,
    metadata          JSONB NOT NULL DEFAULT '{}',
    idempotency_key   TEXT,
    scheduled_at      TIMESTAMPTZ,
    priority          INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    consumed_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    error             TEXT,
    retry_of_outbox_id TEXT,
    archived_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, consumed_at)
) PARTITION BY RANGE (consumed_at);

CREATE TABLE IF NOT EXISTS enqueue_outbox_history_default
    PARTITION OF enqueue_outbox_history DEFAULT;

-- Create partitions for current + next 3 months.
DO $$
DECLARE
    start_date DATE; end_date DATE; partition_name TEXT;
BEGIN
    FOR i IN 0..3 LOOP
        start_date := DATE_TRUNC('month', CURRENT_DATE) + (i || ' months')::INTERVAL;
        end_date := start_date + '1 month'::INTERVAL;
        partition_name := 'enqueue_outbox_history_p' || TO_CHAR(start_date, 'YYYY_MM');
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF enqueue_outbox_history FOR VALUES FROM (%L) TO (%L)',
            partition_name, start_date, end_date
        );
    END LOOP;
END $$;

CREATE INDEX IF NOT EXISTS idx_outbox_history_consumed_at
    ON enqueue_outbox_history (consumed_at);
CREATE INDEX IF NOT EXISTS idx_outbox_history_project
    ON enqueue_outbox_history (project_id, consumed_at DESC);
CREATE INDEX IF NOT EXISTS idx_outbox_history_quarantined
    ON enqueue_outbox_history (project_id, consumed_at DESC)
    WHERE error IS NOT NULL AND error <> '';

ALTER TABLE enqueue_outbox_history SET (fillfactor = 100, autovacuum_vacuum_scale_factor = 0.1);

UPDATE schema_version SET version = 219, updated_at = NOW();
