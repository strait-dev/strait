ALTER TABLE organization_subscriptions
  ADD COLUMN IF NOT EXISTS anomaly_threshold_warning DOUBLE PRECISION DEFAULT 3.0,
  ADD COLUMN IF NOT EXISTS anomaly_threshold_critical DOUBLE PRECISION DEFAULT 10.0;
