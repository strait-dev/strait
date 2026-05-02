-- Maintain job_run_queue atomically from job_runs status transitions.
-- This supplements the application-level dual-write with a database-level
-- trigger that guarantees claim rows are cleaned up when runs leave the
-- queued/delayed state, regardless of which code path modifies the run.
--
-- Performance: the WHEN clause ensures the trigger body only executes when
-- the status column actually changes to or from queued/delayed. Status
-- transitions between other states (dequeued->executing->completed) are
-- the hot path and must NOT fire this trigger.

CREATE OR REPLACE FUNCTION trg_job_runs_sync_claim_queue()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = public, pg_catalog
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        DELETE FROM job_run_queue WHERE run_id = OLD.id;
        RETURN OLD;
    END IF;

    IF TG_OP = 'INSERT' THEN
        IF NEW.status IN ('queued', 'delayed') THEN
            INSERT INTO job_run_queue (
                run_id, job_id, project_id, priority, created_at,
                scheduled_at, next_retry_at, concurrency_key,
                job_max_concurrency, job_max_concurrency_per_key,
                job_enabled, job_paused
            )
            SELECT
                NEW.id, NEW.job_id, NEW.project_id, NEW.priority, NEW.created_at,
                NEW.scheduled_at, NEW.next_retry_at, NEW.concurrency_key,
                j.max_concurrency, j.max_concurrency_per_key,
                j.enabled, j.paused
            FROM jobs j
            WHERE j.id = NEW.job_id
            ON CONFLICT (run_id) DO NOTHING;
        END IF;
        RETURN NEW;
    END IF;

    -- UPDATE: only reached when the WHEN clause fires (status changed
    -- to/from queued/delayed).
    IF NEW.status IN ('queued', 'delayed') THEN
        -- Row entered queued/delayed: upsert claim row.
        INSERT INTO job_run_queue (
            run_id, job_id, project_id, priority, created_at,
            scheduled_at, next_retry_at, concurrency_key,
            job_max_concurrency, job_max_concurrency_per_key,
            job_enabled, job_paused
        )
        SELECT
            NEW.id, NEW.job_id, NEW.project_id, NEW.priority, NEW.created_at,
            NEW.scheduled_at, NEW.next_retry_at, NEW.concurrency_key,
            j.max_concurrency, j.max_concurrency_per_key,
            j.enabled, j.paused
        FROM jobs j
        WHERE j.id = NEW.job_id
        ON CONFLICT (run_id) DO UPDATE SET
            priority = EXCLUDED.priority,
            scheduled_at = EXCLUDED.scheduled_at,
            next_retry_at = EXCLUDED.next_retry_at,
            concurrency_key = EXCLUDED.concurrency_key,
            job_max_concurrency = EXCLUDED.job_max_concurrency,
            job_max_concurrency_per_key = EXCLUDED.job_max_concurrency_per_key,
            job_enabled = EXCLUDED.job_enabled,
            job_paused = EXCLUDED.job_paused;
    ELSE
        -- Row left queued/delayed: remove claim row.
        DELETE FROM job_run_queue WHERE run_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

-- The WHEN clause restricts the trigger to fire ONLY when:
-- 1. INSERT: always (need to seed claim rows for new queued/delayed runs)
-- 2. DELETE: always (cleanup)
-- 3. UPDATE: only when status changes to/from queued/delayed
-- This ensures the hot path (dequeued->executing->completed) never fires
-- the trigger, which is critical for performance.
CREATE TRIGGER trg_job_runs_claim_queue_sync
    AFTER INSERT OR DELETE ON job_runs
    FOR EACH ROW
    EXECUTE FUNCTION trg_job_runs_sync_claim_queue();

CREATE TRIGGER trg_job_runs_claim_queue_sync_update
    AFTER UPDATE ON job_runs
    FOR EACH ROW
    WHEN (
        -- Status changed to/from queued/delayed.
        (OLD.status IS DISTINCT FROM NEW.status
         AND (OLD.status IN ('queued', 'delayed')
              OR NEW.status IN ('queued', 'delayed')))
        -- OR: queue-ordering fields changed while row stays queued/delayed.
        OR (NEW.status IN ('queued', 'delayed')
            AND (OLD.priority IS DISTINCT FROM NEW.priority
                 OR OLD.scheduled_at IS DISTINCT FROM NEW.scheduled_at
                 OR OLD.next_retry_at IS DISTINCT FROM NEW.next_retry_at
                 OR OLD.concurrency_key IS DISTINCT FROM NEW.concurrency_key))
    )
    EXECUTE FUNCTION trg_job_runs_sync_claim_queue();
