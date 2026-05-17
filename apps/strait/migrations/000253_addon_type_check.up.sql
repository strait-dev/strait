-- CHECK constraint on organization_addons.addon_type. Includes the 6 canonical
-- Notion-catalog addons plus 5 deprecated legacy types that are still
-- referenced by tests until Phase 2b removes them. Phase 2b will tighten this
-- constraint to canonical-only.

ALTER TABLE organization_addons
    ADD CONSTRAINT organization_addons_addon_type_check
    CHECK (addon_type IN (
        -- Canonical (Notion catalog)
        'concurrency_100',
        'log_drain_10gb',
        'history_30d',
        'compliance_archive',
        'dedicated_workers',
        'environments_5',
        -- Deprecated; removed in Phase 2b
        'concurrent_runs',
        'members',
        'cron_schedules',
        'data_retention',
        'webhook_endpoints'
    ));
