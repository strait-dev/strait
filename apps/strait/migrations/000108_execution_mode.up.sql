-- Add execution mode columns for managed container execution.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS execution_mode TEXT NOT NULL DEFAULT 'http'
    CHECK (execution_mode IN ('http', 'managed'));
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS machine_preset TEXT
    CHECK (machine_preset IS NULL OR machine_preset IN ('micro', 'small-1x', 'small-2x', 'medium-1x', 'medium-2x', 'large-1x', 'large-2x'));
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS image_uri TEXT;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS region TEXT;
