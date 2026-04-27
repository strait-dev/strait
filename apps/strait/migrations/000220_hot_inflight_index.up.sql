-- Create the HOT-friendly replacement index for stale-dequeued and
-- stale-executing reaper queries. Does NOT predicate on status, so
-- status transitions between 'dequeued' and 'executing' become
-- HOT-eligible.
--
-- Single-statement file so golang-migrate sends it outside a
-- transaction (required for CONCURRENTLY).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_inflight_started
  ON job_runs (started_at ASC)
  WHERE finished_at IS NULL AND started_at IS NOT NULL;
