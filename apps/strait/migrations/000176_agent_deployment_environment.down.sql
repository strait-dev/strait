DROP INDEX IF EXISTS idx_agent_deployments_agent_env;
ALTER TABLE agent_deployments DROP COLUMN IF EXISTS environment_id;
