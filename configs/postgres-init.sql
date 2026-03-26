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

-- Set replica identity to full for CDC tables so Sequin includes
-- the changes field in message payloads (shows which columns changed).
-- These run after Strait migrations create the tables, so they are
-- idempotent and safe to re-run.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'event_triggers') THEN
        ALTER TABLE public.event_triggers REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'workflow_runs') THEN
        ALTER TABLE public.workflow_runs REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'workflow_step_runs') THEN
        ALTER TABLE public.workflow_step_runs REPLICA IDENTITY FULL;
    END IF;
END
$$;
