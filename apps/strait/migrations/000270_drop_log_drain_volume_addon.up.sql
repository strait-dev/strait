-- LogDrainVolumeGB was a catalog placeholder that was never enforced — the
-- enforcement.go branch that should have read it was explicitly discarded
-- with `_ = addOns.LogDrainVolumeGB`. Strip the key from any existing
-- organization_subscriptions.add_ons JSONB and refresh the column comment
-- so the documented schema matches the live struct.

UPDATE organization_subscriptions
SET add_ons = add_ons - 'log_drain_volume_gb'
WHERE add_ons ? 'log_drain_volume_gb';

COMMENT ON COLUMN organization_subscriptions.add_ons IS
    'Per-org subscription add-ons. Schema: {"retention_pack": int, "priority_slot_pack": int, "worker_connections": int}';
