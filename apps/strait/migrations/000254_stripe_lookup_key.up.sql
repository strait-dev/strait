-- Add stripe_lookup_key columns so the Phase 4 lookup-key resolver can
-- record which Stripe price (cross-account stable identifier) backs each
-- subscription / addon row, independent of the per-account price ID.
-- Indexes for these columns are created CONCURRENTLY in follow-up
-- migrations 255 and 256.

ALTER TABLE organization_subscriptions
    ADD COLUMN IF NOT EXISTS stripe_lookup_key TEXT;

ALTER TABLE organization_addons
    ADD COLUMN IF NOT EXISTS stripe_lookup_key TEXT;
