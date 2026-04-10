-- Bind agent deployments to an environment so the platform environments
-- primitive serves Agents the same way Job.EnvironmentID serves Jobs.
--
-- Nullable on rollout because existing deployments were created before
-- environments existed on the Agents side. New deployments should always
-- carry environment_id (enforced at the API layer).
ALTER TABLE agent_deployments
    ADD COLUMN environment_id TEXT REFERENCES environments(id) ON DELETE SET NULL;

-- Supports "what's active in this env?" lookups used by RunAgent and the
-- canary router.
CREATE INDEX IF NOT EXISTS idx_agent_deployments_agent_env
    ON agent_deployments (agent_id, environment_id)
    WHERE environment_id IS NOT NULL;
