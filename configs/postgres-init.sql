-- Create logical replication slot and publication for Sequin CDC.
-- This runs on first Postgres boot (via docker-entrypoint-initdb.d).

-- Create replication slot if not exists.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = 'sequin_strait_slot') THEN
        PERFORM pg_create_logical_replication_slot('sequin_strait_slot', 'pgoutput');
    END IF;
END
$$;

-- Create publication for all tables (Sequin will filter).
CREATE PUBLICATION sequin_strait_pub FOR ALL TABLES;
