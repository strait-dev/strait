ALTER TABLE organization_subscriptions
    ADD COLUMN IF NOT EXISTS add_ons JSONB NOT NULL DEFAULT '{}'::JSONB;

COMMENT ON COLUMN organization_subscriptions.add_ons IS
    'Per-org subscription add-ons. Schema: {"retention_pack": int, "priority_slot_pack": int, "log_drain_volume_gb": int, "worker_connections": int}';
