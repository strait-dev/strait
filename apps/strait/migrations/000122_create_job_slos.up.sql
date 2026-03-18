CREATE TABLE job_slos (
    id          TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    job_id      TEXT        NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    project_id  TEXT        NOT NULL,
    metric      TEXT        NOT NULL CHECK (metric IN ('success_rate', 'p95_latency_secs', 'p99_latency_secs')),
    target      DOUBLE PRECISION NOT NULL,
    window_hours INT       NOT NULL CHECK (window_hours IN (24, 168, 720)),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(job_id, metric, window_hours)
);

CREATE TABLE job_slo_evaluations (
    id          TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    slo_id      TEXT        NOT NULL REFERENCES job_slos(id) ON DELETE CASCADE,
    current_value DOUBLE PRECISION NOT NULL,
    budget_remaining DOUBLE PRECISION NOT NULL,
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_job_slo_evaluations_slo_id ON job_slo_evaluations(slo_id, evaluated_at DESC);
