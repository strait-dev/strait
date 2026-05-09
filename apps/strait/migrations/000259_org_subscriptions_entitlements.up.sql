-- Snapshot the resolved entitlements (plan limits + active addons) onto each
-- organization_subscriptions row so quota checks on the hot path can read a
-- single column instead of joining org_subscriptions, organization_addons,
-- and the in-process plan catalog. Webhook handlers and subscription mutators
-- are responsible for keeping this column in sync; readers treat it as
-- authoritative for runtime quota decisions.
--
-- The default '{}' makes the column safe to add without backfill: callers
-- that haven't been updated yet keep working, and the resolver fills it in
-- on the next subscription change. A backfill of existing rows can be done
-- in a follow-up data migration once the writer side is wired up.

ALTER TABLE organization_subscriptions
    ADD COLUMN IF NOT EXISTS entitlements JSONB NOT NULL DEFAULT '{}'::jsonb;
