-- Revert canonical-only CHECK to the broader set used in 000253.

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
