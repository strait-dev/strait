-- Maintain job_run_queue atomically from job_runs status transitions.
-- This replaces the application-level best-effort dual-write with a
-- database-level trigger that guarantees the claim table is always
-- consistent with job_runs, regardless of which code path modifies
-- the run (API, scheduler, reaper, replay, etc.).
--
-- Rules:
--   INSERT with status IN ('queued','delayed') -> INSERT claim row
--   UPDATE to status IN ('queued','delayed')   -> UPSERT claim row
--   UPDATE away from ('queued','delayed')      -> DELETE claim row
--   DELETE                                      -> DELETE claim row

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

    -- UPDATE: check if the row should be in the claim table.
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
        -- Row is leaving queued/delayed state; remove from claim table.
        DELETE FROM job_run_queue WHERE run_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_job_runs_claim_queue_sync
    AFTER INSERT OR UPDATE OR DELETE ON job_runs
    FOR EACH ROW
    EXECUTE FUNCTION trg_job_runs_sync_claim_queue();
