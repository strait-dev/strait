ALTER TABLE organization_subscriptions
    DROP COLUMN IF EXISTS cap_warning_dispatched_at,
    DROP COLUMN IF EXISTS cap_reached_dispatched_at,
    DROP COLUMN IF EXISTS cap_disabled_dispatched_at,
    DROP COLUMN IF EXISTS overage_disabled_dispatched_at;
