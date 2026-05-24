CREATE TABLE IF NOT EXISTS workflow_progression_events (
    id BIGSERIAL PRIMARY KEY,
    workflow_run_id TEXT NOT NULL,
    step_run_id TEXT NOT NULL,
    step_ref TEXT NOT NULL,
    status TEXT NOT NULL,
    locked_at TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (step_run_id, status)
);

CREATE INDEX IF NOT EXISTS idx_workflow_progression_events_claim
    ON workflow_progression_events(created_at ASC)
    WHERE processed_at IS NULL;
