-- Partial index over stripe_lookup_key for active-addon resolver lookups.
-- CONCURRENTLY for the same reason as migration 255.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_org_addons_stripe_lookup_key
    ON organization_addons(stripe_lookup_key)
    WHERE stripe_lookup_key IS NOT NULL AND active = true;
