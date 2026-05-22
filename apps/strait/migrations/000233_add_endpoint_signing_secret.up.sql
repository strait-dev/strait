-- Add endpoint_signing_secret column to jobs for signing outbound HTTP dispatch requests.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS endpoint_signing_secret TEXT NOT NULL DEFAULT '';
