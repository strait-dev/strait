CREATE TABLE IF NOT EXISTS workflow_progression_event_claims (
    event_id  BIGINT PRIMARY KEY REFERENCES workflow_progression_events(id) ON DELETE CASCADE,
    locked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempts  INT NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS workflow_progression_event_processed (
    event_id     BIGINT PRIMARY KEY REFERENCES workflow_progression_events(id) ON DELETE CASCADE,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- safety-ok: new side table; the index is created before application writes can
-- accumulate, and golang-migrate wraps migrations so CONCURRENTLY is not usable.
CREATE INDEX IF NOT EXISTS idx_workflow_progression_event_processed_cleanup
    ON workflow_progression_event_processed(processed_at ASC, event_id ASC);

INSERT INTO workflow_progression_event_claims (event_id, locked_at, attempts)
SELECT id, locked_at, GREATEST(attempts, 1)
FROM workflow_progression_events
WHERE processed_at IS NULL
  AND locked_at IS NOT NULL
ON CONFLICT (event_id) DO NOTHING;

INSERT INTO workflow_progression_event_processed (event_id, processed_at)
SELECT id, processed_at
FROM workflow_progression_events
WHERE processed_at IS NOT NULL
ON CONFLICT (event_id) DO NOTHING;
