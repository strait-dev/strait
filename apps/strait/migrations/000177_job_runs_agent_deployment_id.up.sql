-- Stamp job runs with the originating agent deployment (when produced via
-- an agent backing job) so per-deployment concurrency, secrets resolution,
-- and replay targeting can all key off a single identifier.
--
-- This column is distinct from job_runs.deployment_id which was added by
-- migration 000170 as an FK to code_deployments(id) for code-first job
-- deployments. The agent backing-job path is a separate concept.
--
-- No FK: job_runs is range-partitioned by created_at and cross-partition
-- FKs are awkward; integrity is enforced at the application layer, matching
-- the existing parent_run_id pattern.
ALTER TABLE job_runs
    ADD COLUMN agent_deployment_id TEXT;

-- Partial index scoped to rows that actually belong to an agent
-- deployment. Keeps the index small while still covering the
-- ListRunsByJobAndAgentDeployment concurrency query.
CREATE INDEX IF NOT EXISTS idx_job_runs_job_agent_deployment
    ON job_runs (job_id, agent_deployment_id)
    WHERE agent_deployment_id IS NOT NULL;
