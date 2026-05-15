-- Revert canonical-only CHECK to the broader set used in 000253.
--
-- The defensive DELETE below removes any rows whose addon_type sits outside
-- the broader (legacy + canonical) set. Direct DB writes or partially-applied
-- migrations could leave such values in place; without the cleanup the ADD
-- CONSTRAINT below would fail and trap the rollback half-way.

DELETE FROM organization_addons
WHERE addon_type NOT IN (
    'concurrency_100',
    'log_drain_10gb',
    'history_30d',
    'compliance_archive',
    'dedicated_workers',
    'environments_5',
    'concurrent_runs',
    'members',
    'cron_schedules',
    'data_retention',
    'webhook_endpoints'
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
        'environments_5',
        'concurrent_runs',
        'members',
        'cron_schedules',
        'data_retention',
        'webhook_endpoints'
    ));
