-- Create logical replication slot and publication for Sequin CDC.
-- This runs on first Postgres boot (via docker-entrypoint-initdb.d).

-- Runtime role used by security probes and RLS/audit privilege hardening.
-- Migrations grant and revoke privileges for this role when it exists, so
-- create it before Strait applies migrations in local/perf/self-host stacks.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'strait_app') THEN
        CREATE ROLE strait_app NOLOGIN NOBYPASSRLS;
    END IF;
END
$$;

-- Create replication slot if not exists.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_replication_slots WHERE slot_name = 'sequin_strait_slot') THEN
        PERFORM pg_create_logical_replication_slot('sequin_strait_slot', 'pgoutput');
    END IF;
END
$$;

-- Create publication for all tables (Sequin will filter).
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_publication WHERE pubname = 'sequin_strait_pub') THEN
        CREATE PUBLICATION sequin_strait_pub FOR ALL TABLES WITH (publish_via_partition_root = true);
    END IF;
END
$$;

ALTER PUBLICATION sequin_strait_pub SET (publish_via_partition_root = true);

-- Set replica identity to full for CDC tables so Sequin includes
-- the changes field in message payloads (shows which columns changed).
-- These run after Strait migrations create the tables, so they are
-- idempotent and safe to re-run.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'api_keys') THEN
        ALTER TABLE public.api_keys REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'project_roles') THEN
        ALTER TABLE public.project_roles REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'project_member_roles') THEN
        ALTER TABLE public.project_member_roles REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'resource_policies') THEN
        ALTER TABLE public.resource_policies REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'tag_policies') THEN
        ALTER TABLE public.tag_policies REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'project_quotas') THEN
        ALTER TABLE public.project_quotas REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'organization_subscriptions') THEN
        ALTER TABLE public.organization_subscriptions REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'jobs') THEN
        ALTER TABLE public.jobs REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'job_dependencies') THEN
        ALTER TABLE public.job_dependencies REPLICA IDENTITY FULL;
    END IF;
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'job_runs') THEN
        ALTER TABLE public.job_runs REPLICA IDENTITY FULL;
    END IF;
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
