-- Restore the previous comment shape. Existing rows that had the key
-- stripped are not re-populated — there is no enforcement code to support
-- a rollback to LogDrainVolumeGB.

COMMENT ON COLUMN organization_subscriptions.add_ons IS
    'Per-org subscription add-ons. Schema: {"retention_pack": int, "priority_slot_pack": int, "log_drain_volume_gb": int, "worker_connections": int}';
