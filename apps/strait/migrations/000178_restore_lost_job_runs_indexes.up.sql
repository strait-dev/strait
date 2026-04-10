-- Restore indexes lost when migration 000066 rebuilt job_runs as a
-- partitioned table. The partitioning migration renamed the old table to
-- job_runs_legacy, created a fresh partitioned table, and recreated only
-- five of the indexes it had explicitly dropped by name. Every other index
-- that existed on the legacy table was silently destroyed when the legacy
-- table was dropped via CASCADE.
--
-- These four indexes existed in migrations 000006 and 000007 and were
-- critical to the dequeue and reaper hot paths. They have been missing
-- from the partitioned table since the partitioning rollout.

-- ListRunsByJob and ListRunsByProject with a job filter.
-- Also backs the keyset pagination added later for ListRunsByJobKeyset.
-- Originally created in 000006 as idx_runs_job_id.
CREATE INDEX IF NOT EXISTS idx_job_runs_job_id_created
    ON job_runs (job_id, created_at DESC);

-- Dequeue retry backoff predicate. Without this index, every dequeue
-- evaluates (next_retry_at IS NULL OR next_retry_at <= NOW()) against
-- every queued row. Originally created in 000006 as idx_runs_retry.
CREATE INDEX IF NOT EXISTS idx_job_runs_retry
    ON job_runs (next_retry_at)
    WHERE status = 'queued' AND next_retry_at IS NOT NULL;

-- Stale dequeued reaper query: ListStaleDequeued in internal/store/runs.go
-- filters WHERE status = 'dequeued' AND started_at < NOW() - interval and
-- orders by started_at ASC. Originally created in 000006 as
-- idx_runs_status_dequeued.
CREATE INDEX IF NOT EXISTS idx_job_runs_stale_dequeued
    ON job_runs (started_at)
    WHERE status = 'dequeued';

-- Dequeue priority sort: internal/queue/postgres.go hardcodes
-- ORDER BY jr.priority DESC, jr.created_at ASC on the SKIP LOCKED candidate
-- selection. The existing idx_runs_queue_covering (from 000068) has
-- leading column created_at and can only scan + filter; Postgres cannot
-- eliminate the sort because the key order does not match. This partial
-- index lets the dequeue walk rows in the exact ORDER BY order.
-- Originally created in 000007 as idx_runs_priority.
CREATE INDEX IF NOT EXISTS idx_job_runs_queue_priority
    ON job_runs (priority DESC, created_at ASC)
    WHERE status = 'queued';
