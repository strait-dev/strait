-- Partial index covering only rows in an active dunning cycle. CONCURRENTLY
-- avoids locking organization_subscriptions; lives in its own migration file
-- because golang-migrate wraps each multi-statement file in a transaction and
-- CREATE INDEX CONCURRENTLY cannot run inside one.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_org_subscriptions_dunning_active
    ON organization_subscriptions (dunning_entered_at)
    WHERE dunning_entered_at IS NOT NULL AND dunning_resolved_at IS NULL;
