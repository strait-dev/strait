CREATE TABLE IF NOT EXISTS organization_addons (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    org_id TEXT NOT NULL,
    addon_type TEXT NOT NULL,
    quantity INT NOT NULL DEFAULT 1,
    polar_subscription_id TEXT,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_org_addons_unique
    ON organization_addons (org_id, addon_type, polar_subscription_id)
    WHERE active = true;

CREATE INDEX IF NOT EXISTS idx_org_addons_org_id
    ON organization_addons (org_id)
    WHERE active = true;
