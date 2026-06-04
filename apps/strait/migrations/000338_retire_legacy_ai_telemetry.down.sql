DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'cost_stats_hourly'
          AND column_name = 'deprecated_token_count'
    ) THEN
        ALTER TABLE cost_stats_hourly RENAME COLUMN deprecated_token_count TO total_tokens;
    ELSE
        ALTER TABLE cost_stats_hourly ADD COLUMN IF NOT EXISTS total_tokens BIGINT NOT NULL DEFAULT 0;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS pricing_catalog (
    id                      TEXT        PRIMARY KEY,
    provider                TEXT        NOT NULL,
    model                   TEXT        NOT NULL,
    input_cost_microusd     BIGINT      NOT NULL,
    output_cost_microusd    BIGINT      NOT NULL,
    active                  BOOLEAN     NOT NULL DEFAULT TRUE,
    effective_from          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, model, effective_from)
);

CREATE INDEX IF NOT EXISTS idx_pricing_catalog_provider_model_active
    ON pricing_catalog (provider, model, active, effective_from DESC);

CREATE TABLE IF NOT EXISTS run_usage (
    id                  TEXT        PRIMARY KEY,
    run_id              TEXT        NOT NULL,
    provider            TEXT        NOT NULL,
    model               TEXT        NOT NULL,
    prompt_tokens       INT         NOT NULL DEFAULT 0,
    completion_tokens   INT         NOT NULL DEFAULT 0,
    total_tokens        INT         NOT NULL DEFAULT 0,
    cost_microusd       BIGINT      NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_run_usage_run_id_created_at
    ON run_usage (run_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_run_usage_created_at
    ON run_usage (created_at);

CREATE TABLE IF NOT EXISTS run_tool_calls (
    id                TEXT        PRIMARY KEY,
    run_id            TEXT        NOT NULL,
    tool_name         TEXT        NOT NULL,
    input             JSONB,
    output            JSONB,
    duration_ms       INT,
    status            TEXT        NOT NULL DEFAULT 'completed',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_run_tool_calls_run_id_created_at
    ON run_tool_calls (run_id, created_at DESC);
