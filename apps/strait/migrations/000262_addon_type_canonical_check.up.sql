-- Tighten organization_addons.addon_type CHECK to canonical-only. Phase 2b
-- removed the 5 deprecated addon types (concurrent_runs, members,
-- cron_schedules, data_retention, webhook_endpoints) from application code,
-- so the constraint added in 000253 can be replaced with the canonical set.
--
-- The defensive DELETE below removes any deprecated rows still living in the
-- table before the tighter constraint is rebuilt. The application stopped
-- writing those types in Phase 2b, so any survivors are stale records the
-- catalog no longer prices; without the cleanup the ADD CONSTRAINT below
-- would fail on rollouts that happen to lag behind the application change.

DELETE FROM organization_addons
WHERE addon_type NOT IN (
    'concurrency_100',
    'log_drain_10gb',
    'history_30d',
    'compliance_archive',
    'dedicated_workers',
    'environments_5'
);

ALTER TABLE organization_addons DROP CONSTRAINT IF EXISTS organization_addons_addon_type_check;

ALTER TABLE organization_addons
    ADD CONSTRAINT organization_addons_addon_type_check
    CHECK (addon_type IN (
        'concurrency_100',
        'log_drain_10gb',
        'history_30d',
        'compliance_archive',
        'dedicated_workers',
        'environments_5'
    ));
