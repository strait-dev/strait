CREATE TABLE IF NOT EXISTS notify_policy_overrides (
    id                              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    project_id                      TEXT NOT NULL,
    scope_type                      TEXT NOT NULL,
    scope_key                       TEXT NOT NULL,
    channel                         TEXT NOT NULL DEFAULT '',
    digest_policy                   TEXT,
    retry_max_attempts              INT,
    retry_base_delay_secs           INT,
    retry_max_delay_secs            INT,
    escalation_tiers                INT,
    escalation_min_interval_secs    INT,
    enabled                         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (scope_type IN ('project', 'category', 'workflow_step')),
    CHECK (digest_policy IS NULL OR digest_policy IN ('instant', 'hourly', 'daily')),
    CHECK (retry_max_attempts IS NULL OR retry_max_attempts > 0),
    CHECK (retry_base_delay_secs IS NULL OR retry_base_delay_secs > 0),
    CHECK (retry_max_delay_secs IS NULL OR retry_max_delay_secs > 0),
    CHECK (escalation_tiers IS NULL OR escalation_tiers > 0),
    CHECK (escalation_min_interval_secs IS NULL OR escalation_min_interval_secs > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notify_policy_overrides_scope
    ON notify_policy_overrides(project_id, scope_type, scope_key, channel);

CREATE INDEX IF NOT EXISTS idx_notify_policy_overrides_project
    ON notify_policy_overrides(project_id, scope_type, enabled);
