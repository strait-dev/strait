DROP TABLE IF EXISTS debounce_pending;
ALTER TABLE jobs DROP COLUMN IF EXISTS debounce_window_secs;
