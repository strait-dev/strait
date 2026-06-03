DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'cost_stats_hourly'
          AND column_name = 'usage_cost_microusd'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'cost_stats_hourly'
          AND column_name = 'ai_cost_microusd'
    ) THEN
        ALTER TABLE cost_stats_hourly RENAME COLUMN usage_cost_microusd TO ai_cost_microusd;
    END IF;
END $$;
