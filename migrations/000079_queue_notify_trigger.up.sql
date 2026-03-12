CREATE OR REPLACE FUNCTION notify_queue_wake() RETURNS trigger AS $$
BEGIN
  IF NEW.status = 'queued' AND (TG_OP = 'INSERT' OR OLD.status IS DISTINCT FROM NEW.status) THEN
    PERFORM pg_notify('strait_queue_wake', NEW.id);
  END IF;

  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_job_runs_queue_wake_notify ON job_runs;

CREATE TRIGGER trg_job_runs_queue_wake_notify
AFTER INSERT OR UPDATE OF status ON job_runs
FOR EACH ROW
EXECUTE FUNCTION notify_queue_wake();
