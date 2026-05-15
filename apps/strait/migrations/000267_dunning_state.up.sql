ALTER TABLE organization_subscriptions
    ADD COLUMN dunning_step          SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN dunning_entered_at    TIMESTAMPTZ,
    ADD COLUMN dunning_last_tick_at  TIMESTAMPTZ,
    ADD COLUMN dunning_resolved_at   TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_org_subscriptions_dunning_active
    ON organization_subscriptions (dunning_entered_at)
    WHERE dunning_entered_at IS NOT NULL AND dunning_resolved_at IS NULL;

COMMENT ON COLUMN organization_subscriptions.dunning_step IS
    'Dunning progression step: 0=not in dunning, 1=Day 0 entry, 2=Day 3, 3=Day 7, 4=Day 14 (restricted), 5=Day 44 (final warning), 6=Day 74 (suspended).';
COMMENT ON COLUMN organization_subscriptions.dunning_entered_at IS
    'Timestamp at which the current dunning cycle started. NULL when not in dunning.';
COMMENT ON COLUMN organization_subscriptions.dunning_last_tick_at IS
    'Timestamp at which the Dunner last processed this row. Drives the 24h per-row cooldown.';
COMMENT ON COLUMN organization_subscriptions.dunning_resolved_at IS
    'Timestamp at which the dunning cycle was resolved (e.g. invoice.paid). NULL for active cycles.';
