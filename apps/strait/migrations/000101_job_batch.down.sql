DROP TABLE IF EXISTS batch_buffer;
ALTER TABLE jobs DROP COLUMN IF EXISTS batch_max_size;
ALTER TABLE jobs DROP COLUMN IF EXISTS batch_window_secs;
