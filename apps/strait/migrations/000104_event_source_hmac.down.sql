ALTER TABLE event_sources
  DROP COLUMN IF EXISTS signature_header,
  DROP COLUMN IF EXISTS signature_algorithm,
  DROP COLUMN IF EXISTS signature_secret_enc;
