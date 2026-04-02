CREATE TABLE IF NOT EXISTS enterprise_contracts (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    org_id TEXT NOT NULL,
    enterprise_tier TEXT NOT NULL CHECK (enterprise_tier IN ('enterprise_starter', 'enterprise_growth', 'enterprise_large')),
    annual_commitment_cents BIGINT NOT NULL,
    included_credit_microusd BIGINT NOT NULL,
    compute_discount_pct INTEGER NOT NULL DEFAULT 0,
    contract_start_date TIMESTAMPTZ NOT NULL,
    contract_end_date TIMESTAMPTZ NOT NULL,
    auto_renew BOOLEAN NOT NULL DEFAULT true,
    billing_cadence TEXT NOT NULL DEFAULT 'annual' CHECK (billing_cadence IN ('annual', 'quarterly')),
    stripe_subscription_id TEXT,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id)
);

CREATE INDEX IF NOT EXISTS idx_enterprise_contracts_org ON enterprise_contracts(org_id);
CREATE INDEX IF NOT EXISTS idx_enterprise_contracts_end ON enterprise_contracts(contract_end_date);
