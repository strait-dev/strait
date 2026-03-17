-- Track which region the container actually ran in.
ALTER TABLE run_compute_usage ADD COLUMN IF NOT EXISTS region TEXT NOT NULL DEFAULT '';
