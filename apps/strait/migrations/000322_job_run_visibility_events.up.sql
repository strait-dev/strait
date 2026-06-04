CREATE TABLE IF NOT EXISTS job_run_visibility_events (
    id            BIGSERIAL PRIMARY KEY,
    run_id        TEXT NOT NULL,
    visible_until TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_job_run_visibility_events_latest
    ON job_run_visibility_events(run_id, id DESC);

INSERT INTO job_run_visibility_events (run_id, visible_until, created_at)
SELECT jr.id, jr.visible_until, NOW()
FROM job_runs jr
WHERE jr.visible_until IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM job_run_visibility_events e
      WHERE e.run_id = jr.id
  );
