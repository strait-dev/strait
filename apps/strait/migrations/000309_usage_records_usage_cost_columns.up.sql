-- safety-ok: launch branch rename before public release; application code and OpenAPI were updated in the same branch.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'usage_records'
          AND column_name = 'ai_tokens_total'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'usage_records'
          AND column_name = 'usage_tokens_total'
    ) THEN
        ALTER TABLE usage_records RENAME COLUMN ai_tokens_total TO usage_tokens_total;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'usage_records'
          AND column_name = 'ai_cost_microusd'
    ) AND NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'usage_records'
          AND column_name = 'usage_cost_microusd'
    ) THEN
        ALTER TABLE usage_records RENAME COLUMN ai_cost_microusd TO usage_cost_microusd;
    END IF;
END $$;
