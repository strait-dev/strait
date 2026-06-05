UPDATE organization_subscriptions
SET add_ons = add_ons - 'retention_pack' - 'worker_connections'
WHERE add_ons ?| ARRAY['retention_pack', 'worker_connections'];

COMMENT ON COLUMN organization_subscriptions.add_ons IS
    'Legacy subscription add-ons payload retained for backward compatibility. Launch add-ons live in organization_addons and this JSONB column is not applied to entitlements.';
