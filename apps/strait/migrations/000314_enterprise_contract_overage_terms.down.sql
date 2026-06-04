DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'enterprise_contracts'
          AND column_name = 'included_credit_microusd'
    ) THEN
        ALTER TABLE enterprise_contracts
            ADD COLUMN included_credit_microusd BIGINT NOT NULL DEFAULT 0;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'enterprise_contracts'
          AND column_name = 'compute_discount_pct'
    ) AND EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'enterprise_contracts'
          AND column_name = 'overage_discount_pct'
    ) THEN
        ALTER TABLE enterprise_contracts
            RENAME COLUMN overage_discount_pct TO compute_discount_pct;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'enterprise_contracts'
          AND column_name = 'overage_discount_pct'
    ) THEN
        ALTER TABLE enterprise_contracts
            DROP COLUMN overage_discount_pct;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'enterprise_contracts'
          AND column_name = 'compute_discount_pct'
    ) THEN
        ALTER TABLE enterprise_contracts
            ADD COLUMN compute_discount_pct INTEGER NOT NULL DEFAULT 0;
    END IF;
END $$;
