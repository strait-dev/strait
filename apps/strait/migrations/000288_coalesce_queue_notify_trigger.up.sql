DROP TRIGGER IF EXISTS trg_job_runs_queue_wake_notify ON job_runs;
DROP TRIGGER IF EXISTS trg_job_runs_queue_wake_insert_notify ON job_runs;
DROP TRIGGER IF EXISTS trg_job_runs_queue_wake_update_notify ON job_runs;
DROP FUNCTION IF EXISTS notify_queue_wake();
DROP FUNCTION IF EXISTS notify_queue_wake_insert_stmt();
DROP FUNCTION IF EXISTS notify_queue_wake_update_stmt();

CREATE OR REPLACE FUNCTION notify_queue_wake_insert_stmt() RETURNS trigger AS $$
DECLARE
  queued_count bigint;
BEGIN
  SELECT COUNT(*) INTO queued_count
  FROM new_rows
  WHERE status = 'queued';

  IF queued_count > 0 THEN
    PERFORM pg_notify(
      'strait_queue_wake',
      format('insert:%s:%s:%s', txid_current(), clock_timestamp(), queued_count)
    );
  END IF;

  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION notify_queue_wake_update_stmt() RETURNS trigger AS $$
DECLARE
  queued_count bigint;
BEGIN
  SELECT COUNT(*) INTO queued_count
  FROM new_rows n
  JOIN old_rows o ON o.id = n.id AND o.created_at = n.created_at
  WHERE n.status = 'queued'
    AND o.status IS DISTINCT FROM n.status;

  IF queued_count > 0 THEN
    PERFORM pg_notify(
      'strait_queue_wake',
      format('update:%s:%s:%s', txid_current(), clock_timestamp(), queued_count)
    );
  END IF;

  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_job_runs_queue_wake_insert_notify
AFTER INSERT ON job_runs
REFERENCING NEW TABLE AS new_rows
FOR EACH STATEMENT
EXECUTE FUNCTION notify_queue_wake_insert_stmt();

CREATE TRIGGER trg_job_runs_queue_wake_update_notify
AFTER UPDATE ON job_runs
REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows
FOR EACH STATEMENT
EXECUTE FUNCTION notify_queue_wake_update_stmt();
