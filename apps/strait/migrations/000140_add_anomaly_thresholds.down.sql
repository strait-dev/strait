ALTER TABLE organization_subscriptions
  DROP COLUMN IF EXISTS anomaly_threshold_warning,
  DROP COLUMN IF EXISTS anomaly_threshold_critical;
