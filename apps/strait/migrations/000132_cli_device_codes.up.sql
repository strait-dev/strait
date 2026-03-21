CREATE TABLE cli_device_codes (
    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    device_code TEXT NOT NULL UNIQUE,
    user_code TEXT NOT NULL UNIQUE,
    project_id TEXT NOT NULL,
    api_key_id TEXT,
    raw_api_key TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','used','expired')),
    scopes TEXT[] NOT NULL DEFAULT '{}',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cli_device_codes_device_code ON cli_device_codes(device_code);
CREATE INDEX idx_cli_device_codes_expires_at ON cli_device_codes(expires_at);
