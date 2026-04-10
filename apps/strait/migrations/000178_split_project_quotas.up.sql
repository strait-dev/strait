-- Split project_quotas into three per-concern tables:
--
--   * project_platform_settings — product-neutral platform fields
--   * project_job_quotas        — Jobs-only quotas and limits
--   * project_agent_quotas      — Agents-only quotas and limits
--
-- Motivation: Jobs and Agents are two separately subscribable products
-- that share platform primitives (projects, environments). Mixing their
-- quota columns in one row hid which fields belonged to which product and
-- made it easy for enforcement code to read Jobs limits on an Agents-only
-- customer (and vice versa). This split makes ownership unambiguous and
-- keeps the platform fields in a neutral home.
--
-- This is phase C.1: additive only. We create the new tables and backfill
-- from project_quotas. The old table stays in place so code can be cut
-- over incrementally. A later migration will drop project_quotas.
--
-- Note: plan_tier is intentionally omitted from the backfill. That column
-- is write-dead (nothing in Go ever writes it, every read returns '') —
-- the real source of truth is organization_subscriptions.plan_tier via
-- the billing enforcer. See Phase A's getProjectPlanTierCtx rewrite.

-- Platform-neutral fields: timezone, default_region, rate limits, budget,
-- API key lifetime. These belong to any product running under a project.
CREATE TABLE IF NOT EXISTS project_platform_settings (
    project_id                TEXT        PRIMARY KEY,
    timezone                  TEXT,
    default_region            TEXT,
    max_key_lifetime_days     INT         NOT NULL DEFAULT 0,
    rate_limit_requests       INT,
    rate_limit_window_secs    INT,
    monthly_budget_microusd   BIGINT      DEFAULT -1,
    budget_action             TEXT        DEFAULT 'notify',
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Jobs-only quotas: concurrency, cost, memory.
CREATE TABLE IF NOT EXISTS project_job_quotas (
    project_id                           TEXT        PRIMARY KEY,
    max_jobs                             INT,
    max_queued_runs                      INT,
    max_executing_runs                   INT,
    max_cost_per_run_microusd            BIGINT,
    max_daily_cost_microusd              BIGINT,
    compute_daily_cost_limit_microusd    BIGINT,
    max_memory_per_key_bytes             INT         DEFAULT 1048576,
    max_memory_per_job_bytes             INT         DEFAULT 10485760,
    created_at                           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Agents-only quotas: agent count, run count, channels, guardrails.
-- max_tokens_per_run lands here because agent token accounting is the
-- dominant use case; the one call site on the Jobs side (sdk_telemetry)
-- is flagged for audit in Phase C.2.
CREATE TABLE IF NOT EXISTS project_agent_quotas (
    project_id                   TEXT        PRIMARY KEY,
    max_agents                   INT         NOT NULL DEFAULT 0,
    max_agent_runs_per_month     INT         NOT NULL DEFAULT 0,
    max_agent_channels           INT         NOT NULL DEFAULT 0,
    max_tokens_per_run           BIGINT,
    max_tool_calls_per_run       INT,
    max_iterations_per_run       INT,
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Backfill. Upsert-style ON CONFLICT DO NOTHING so re-running the migration
-- is safe (matches the existing lazy-upsert pattern where callers do
-- ON CONFLICT (project_id) DO UPDATE).
INSERT INTO project_platform_settings (
    project_id, timezone, default_region, max_key_lifetime_days,
    rate_limit_requests, rate_limit_window_secs,
    monthly_budget_microusd, budget_action,
    created_at, updated_at
)
SELECT
    project_id, timezone, default_region,
    COALESCE(max_key_lifetime_days, 0),
    rate_limit_requests, rate_limit_window_secs,
    COALESCE(monthly_budget_microusd, -1),
    COALESCE(budget_action, 'notify'),
    created_at, updated_at
FROM project_quotas
ON CONFLICT (project_id) DO NOTHING;

INSERT INTO project_job_quotas (
    project_id, max_jobs, max_queued_runs, max_executing_runs,
    max_cost_per_run_microusd, max_daily_cost_microusd,
    compute_daily_cost_limit_microusd,
    max_memory_per_key_bytes, max_memory_per_job_bytes,
    created_at, updated_at
)
SELECT
    project_id, max_jobs, max_queued_runs, max_executing_runs,
    max_cost_per_run_microusd, max_daily_cost_microusd,
    compute_daily_cost_limit_microusd,
    COALESCE(max_memory_per_key_bytes, 1048576),
    COALESCE(max_memory_per_job_bytes, 10485760),
    created_at, updated_at
FROM project_quotas
ON CONFLICT (project_id) DO NOTHING;

INSERT INTO project_agent_quotas (
    project_id, max_agents, max_agent_runs_per_month, max_agent_channels,
    max_tokens_per_run, max_tool_calls_per_run, max_iterations_per_run,
    created_at, updated_at
)
SELECT
    project_id,
    COALESCE(max_agents, 0),
    COALESCE(max_agent_runs_per_month, 0),
    COALESCE(max_agent_channels, 0),
    max_tokens_per_run, max_tool_calls_per_run, max_iterations_per_run,
    created_at, updated_at
FROM project_quotas
ON CONFLICT (project_id) DO NOTHING;
