CREATE EXTENSION IF NOT EXISTS pgcrypto;

UPDATE cli_device_codes
SET device_code = 'sha256:' || encode(digest(device_code, 'sha256'), 'hex')
WHERE device_code NOT LIKE 'sha256:%';
