DROP TRIGGER IF EXISTS trg_queue_entries_claimable_wake_insert_notify ON queue_entries;
DROP TRIGGER IF EXISTS trg_queue_entries_claimable_wake_update_notify ON queue_entries;
DROP FUNCTION IF EXISTS notify_queue_entries_claimable_insert_stmt();
DROP FUNCTION IF EXISTS notify_queue_entries_claimable_update_stmt();
