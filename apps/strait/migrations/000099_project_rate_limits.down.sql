ALTER TABLE api_keys
  DROP COLUMN IF EXISTS rate_limit_window_secs,
  DROP COLUMN IF EXISTS rate_limit_requests;

ALTER TABLE project_quotas
  DROP COLUMN IF EXISTS rate_limit_window_secs,
  DROP COLUMN IF EXISTS rate_limit_requests;
