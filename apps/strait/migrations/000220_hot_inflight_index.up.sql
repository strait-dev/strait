-- Create the HOT-friendly replacement index for stale-dequeued and
-- stale-executing reaper queries. Does NOT predicate on status, so
-- status transitions between 'dequeued' and 'executing' become
-- HOT-eligible.
--
-- safety-ok: CONCURRENTLY is not supported on partitioned parent tables in Postgres.
-- The partial index covers only in-flight runs (small subset), so the
-- brief ACCESS EXCLUSIVE lock is acceptable during a maintenance window.
-- Existing partitioned-table indexes (000178, 000197, 000198) follow the
-- same non-concurrent pattern.
CREATE INDEX IF NOT EXISTS idx_job_runs_inflight_started
  ON job_runs (started_at ASC)
  WHERE finished_at IS NULL AND started_at IS NOT NULL;
