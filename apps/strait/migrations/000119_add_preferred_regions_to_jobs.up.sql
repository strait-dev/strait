-- Add preferred_regions to jobs for multi-region configuration.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS preferred_regions TEXT[] DEFAULT '{}';
