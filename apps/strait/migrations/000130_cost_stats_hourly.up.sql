CREATE TABLE cost_stats_hourly (
    project_id            TEXT NOT NULL,
    hour                  TIMESTAMPTZ NOT NULL,
    ai_cost_microusd      BIGINT NOT NULL DEFAULT 0,
    compute_cost_microusd BIGINT NOT NULL DEFAULT 0,
    total_tokens          BIGINT NOT NULL DEFAULT 0,
    run_count             INT NOT NULL DEFAULT 0,
    PRIMARY KEY (project_id, hour)
);
CREATE INDEX idx_cost_stats_hourly_project ON cost_stats_hourly(project_id, hour DESC);
