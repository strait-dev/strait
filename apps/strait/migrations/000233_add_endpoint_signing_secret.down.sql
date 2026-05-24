-- Remove endpoint_signing_secret column from jobs.
ALTER TABLE jobs DROP COLUMN IF EXISTS endpoint_signing_secret;
