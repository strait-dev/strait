ALTER TABLE organization_subscriptions
    ADD COLUMN cap_warning_dispatched_at      TIMESTAMPTZ,
    ADD COLUMN cap_reached_dispatched_at      TIMESTAMPTZ,
    ADD COLUMN cap_disabled_dispatched_at     TIMESTAMPTZ,
    ADD COLUMN overage_disabled_dispatched_at TIMESTAMPTZ;

COMMENT ON COLUMN organization_subscriptions.cap_warning_dispatched_at IS
    'Timestamp at which billing.cap_warning was first dispatched in the current billing period. NULL means not yet dispatched. Reset on current_period_start rollover.';
COMMENT ON COLUMN organization_subscriptions.cap_reached_dispatched_at IS
    'Timestamp at which billing.cap_reached was first dispatched in the current billing period. NULL means not yet dispatched.';
COMMENT ON COLUMN organization_subscriptions.cap_disabled_dispatched_at IS
    'Timestamp at which billing.cap_disabled was first dispatched in the current billing period. NULL means not yet dispatched.';
COMMENT ON COLUMN organization_subscriptions.overage_disabled_dispatched_at IS
    'Timestamp at which billing.overage_disabled was first dispatched in the current billing period. NULL means not yet dispatched.';
