-- safety-ok: launch branch cleanup; application code no longer reads or writes
-- enterprise included-credit or compute-discount fields, and this coordinated
-- migration moves the contract table to public launch overage terminology.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'enterprise_contracts'
          AND column_name = 'overage_discount_pct'
    ) AND EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'enterprise_contracts'
          AND column_name = 'compute_discount_pct'
    ) THEN
        ALTER TABLE enterprise_contracts
            RENAME COLUMN compute_discount_pct TO overage_discount_pct;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'enterprise_contracts'
          AND column_name = 'compute_discount_pct'
    ) THEN
        ALTER TABLE enterprise_contracts
            DROP COLUMN compute_discount_pct;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'enterprise_contracts'
          AND column_name = 'included_credit_microusd'
    ) THEN
        ALTER TABLE enterprise_contracts
            DROP COLUMN included_credit_microusd;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'enterprise_contracts'
          AND column_name = 'overage_discount_pct'
    ) THEN
        ALTER TABLE enterprise_contracts
            ADD COLUMN overage_discount_pct INTEGER NOT NULL DEFAULT 0;
    END IF;
END $$;
