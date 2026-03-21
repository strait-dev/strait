CREATE TABLE referrals (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    referrer_org_id TEXT NOT NULL,
    referral_code TEXT NOT NULL UNIQUE,
    referred_email TEXT,
    referred_org_id TEXT,
    status TEXT NOT NULL DEFAULT 'pending',
    credit_microusd BIGINT DEFAULT 10000000,
    activated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
