CREATE TABLE run_resource_snapshots (
    id               TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    run_id           TEXT NOT NULL,
    cpu_percent      DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_mb        DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_limit_mb  DOUBLE PRECISION NOT NULL DEFAULT 0,
    network_rx_bytes BIGINT NOT NULL DEFAULT 0,
    network_tx_bytes BIGINT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_run_resource_snapshots_run ON run_resource_snapshots(run_id, created_at);
