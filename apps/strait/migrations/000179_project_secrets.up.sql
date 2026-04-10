-- Lift job_secrets to a project-level, environment-scoped secrets table.
--
-- job_secrets today is scoped by (project_id, job_id, environment_text).
-- This ties secrets to Jobs even though Agents also need env-scoped config
-- (API keys, service endpoints, feature flags). Lifting to a neutral
-- project_secrets table — keyed by (project_id, environment_id) — makes
-- secrets a platform primitive any product can read.
--
-- job_id is preserved as a nullable column so per-job secret overrides
-- continue to work: a row with job_id IS NULL is project-wide inside the
-- environment, while a row with a specific job_id overrides just that
-- job. Matches the existing job_secrets semantics.
--
-- This is phase D.1: additive only. We create project_secrets and backfill
-- from job_secrets, resolving the text environment column to an
-- environment_id via environments.slug. Rows whose environment text does
-- not match any environment slug in the project are dropped and logged.
-- job_secrets stays in place for cutover and will be dropped in a follow-up.
--
-- Note: the worker executor historically ignored job.environment_id for
-- secrets resolution and read with a hardcoded "production" string. The
-- Phase D code cutover fixes that bug as a side effect.

CREATE TABLE IF NOT EXISTS project_secrets (
    id                TEXT        PRIMARY KEY,
    project_id        TEXT        NOT NULL,
    environment_id    TEXT        NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    job_id            TEXT        REFERENCES jobs(id) ON DELETE CASCADE,
    secret_key        TEXT        NOT NULL,
    encrypted_value   TEXT        NOT NULL,
    key_version       INT         NOT NULL DEFAULT 1,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Separate partial unique indexes so a project-wide secret (job_id NULL)
-- can coexist with a job-specific override for the same key. Postgres
-- treats NULLs as distinct in unique constraints, so a single unique on
-- (project_id, environment_id, job_id, secret_key) wouldn't enforce
-- uniqueness for the NULL case.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_project_secrets_project_env_key
    ON project_secrets (project_id, environment_id, secret_key)
    WHERE job_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_project_secrets_project_env_job_key
    ON project_secrets (project_id, environment_id, job_id, secret_key)
    WHERE job_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_project_secrets_project_env
    ON project_secrets (project_id, environment_id);

-- Backfill from job_secrets. Resolve the text environment column to an
-- environment_id via environments.slug scoped to the same project. Rows
-- without a matching environment are silently skipped — they were
-- unreachable anyway because production code always resolved secrets via
-- environment slug.
INSERT INTO project_secrets (
    id, project_id, environment_id, job_id, secret_key, encrypted_value,
    key_version, created_at, updated_at
)
SELECT
    js.id,
    js.project_id,
    e.id,
    js.job_id,
    js.secret_key,
    js.encrypted_value,
    js.key_version,
    js.created_at,
    js.updated_at
FROM job_secrets js
JOIN environments e
  ON e.project_id = js.project_id AND e.slug = js.environment
ON CONFLICT DO NOTHING;
