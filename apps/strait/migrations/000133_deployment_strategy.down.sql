ALTER TABLE deployment_versions
    DROP COLUMN IF EXISTS canary_duration,
    DROP COLUMN IF EXISTS canary_percent,
    DROP COLUMN IF EXISTS strategy;
