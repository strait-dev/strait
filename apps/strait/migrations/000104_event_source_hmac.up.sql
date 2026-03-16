ALTER TABLE event_sources
  ADD COLUMN signature_header TEXT,
  ADD COLUMN signature_algorithm TEXT,
  ADD COLUMN signature_secret_enc BYTEA;
