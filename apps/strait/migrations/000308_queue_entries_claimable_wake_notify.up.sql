CREATE OR REPLACE FUNCTION notify_queue_entries_claimable_insert_stmt() RETURNS trigger AS $$
DECLARE
  claimable_count bigint;
BEGIN
  SELECT COUNT(*) INTO claimable_count
  FROM new_rows
  WHERE status = 'ready'
    AND batch_id IS NOT NULL;

  IF claimable_count > 0 THEN
    PERFORM pg_notify(
      'strait_queue_wake',
      format('queue_entries_insert:%s:%s:%s', txid_current(), clock_timestamp(), claimable_count)
    );
  END IF;

  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION notify_queue_entries_claimable_update_stmt() RETURNS trigger AS $$
DECLARE
  claimable_count bigint;
BEGIN
  SELECT COUNT(*) INTO claimable_count
  FROM new_rows n
  JOIN old_rows o ON o.run_id = n.run_id
  WHERE n.status = 'ready'
    AND n.batch_id IS NOT NULL
    AND (o.status IS DISTINCT FROM n.status OR o.batch_id IS DISTINCT FROM n.batch_id);

  IF claimable_count > 0 THEN
    PERFORM pg_notify(
      'strait_queue_wake',
      format('queue_entries_update:%s:%s:%s', txid_current(), clock_timestamp(), claimable_count)
    );
  END IF;

  RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_queue_entries_claimable_wake_insert_notify ON queue_entries;
DROP TRIGGER IF EXISTS trg_queue_entries_claimable_wake_update_notify ON queue_entries;

CREATE TRIGGER trg_queue_entries_claimable_wake_insert_notify
AFTER INSERT ON queue_entries
REFERENCING NEW TABLE AS new_rows
FOR EACH STATEMENT
EXECUTE FUNCTION notify_queue_entries_claimable_insert_stmt();

CREATE TRIGGER trg_queue_entries_claimable_wake_update_notify
AFTER UPDATE ON queue_entries
REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows
FOR EACH STATEMENT
EXECUTE FUNCTION notify_queue_entries_claimable_update_stmt();
