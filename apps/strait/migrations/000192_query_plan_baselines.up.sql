-- R3 Phase 6: query plan baselines for drift detection.
--
-- The scheduler.PlanDriftMonitor captures a baseline (top node type +
-- estimated cost) for each watched query and re-runs EXPLAIN daily.
-- A change in node type (e.g. Index Scan -> Seq Scan) triggers a WARN
-- log and a gauge increment so operators see the drift within a day.

CREATE TABLE IF NOT EXISTS query_plan_baselines (
    query_name     TEXT PRIMARY KEY,
    top_node_type  TEXT NOT NULL,
    est_total_cost FLOAT8 NOT NULL,
    plan_json      JSONB NOT NULL,
    captured_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
