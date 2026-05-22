-- Partial index over stripe_lookup_key for fast resolver lookups.
-- CONCURRENTLY so the build does not lock writes; cannot run inside a
-- transaction (golang-migrate handles this when the file is single-stmt).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_org_subscriptions_stripe_lookup_key
    ON organization_subscriptions(stripe_lookup_key)
    WHERE stripe_lookup_key IS NOT NULL;
