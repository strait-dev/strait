CREATE TABLE endpoint_health_scores (
    endpoint_url        TEXT        PRIMARY KEY,
    health_score        DOUBLE PRECISION NOT NULL DEFAULT 100.0,
    success_rate        DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    timeout_rate        DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    latency_score       DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    total_requests      BIGINT      NOT NULL DEFAULT 0,
    last_latency_ms     DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_endpoint_health_scores_score ON endpoint_health_scores(health_score);
