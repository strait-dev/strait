ALTER TABLE project_quotas
  ADD COLUMN rate_limit_requests INT,
  ADD COLUMN rate_limit_window_secs INT;

ALTER TABLE api_keys
  ADD COLUMN rate_limit_requests INT,
  ADD COLUMN rate_limit_window_secs INT;
