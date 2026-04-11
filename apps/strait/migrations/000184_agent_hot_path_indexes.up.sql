-- Hot-path indexes for the Strait Agents SQL surface.
--
-- Phase E1 of the agents hardening work. These indexes cover four
-- queries that became hot after Phase B added environment-bound
-- deployments and per-deployment concurrency accounting:
--
--   * NextAgentDeploymentVersion does MAX(version) WHERE agent_id = $1
--     on every CreateAgentDeployment call (inside the advisory lock).
--     The composite on (agent_id, version DESC) turns this into an
--     index-only lookup.
--
--   * GetLatestAgentDeployment and GetLatestAgentDeploymentByEnvironment
--     both select ORDER BY version DESC LIMIT 1 and benefit from the
--     same composite.
--
--   * ListAgentMessagesByChain sorts by chain_depth ASC inside a single
--     chain_id. The existing idx_agent_messages_chain only covers the
--     WHERE clause; a composite gives an index-only sort.
--
--   * GetAgentTopologyEdges does a per-project GROUP BY source_agent_id,
--     target_agent_id filtered to non-empty source; a partial composite
--     lets Postgres compute the aggregate from index tuples alone.
--
--   * ListAgentMessagesByAgent is rewritten in Phase E1.5 from
--     WHERE (source_agent_id = $1 OR target_agent_id = $1) into a
--     UNION ALL of two index-backed queries. The existing
--     idx_agent_messages_target covers the target branch; this
--     migration adds the complementary idx_agent_messages_source_created
--     for the source branch.
--
-- All indexes are CREATE IF NOT EXISTS so re-running the migration is
-- safe. They are pure additions; no data is moved and no existing
-- queries change behavior.

CREATE INDEX IF NOT EXISTS idx_agent_deployments_agent_version_desc
    ON agent_deployments (agent_id, version DESC);

CREATE INDEX IF NOT EXISTS idx_agent_messages_chain_depth
    ON agent_messages (chain_id, chain_depth ASC);

CREATE INDEX IF NOT EXISTS idx_agent_messages_project_edges
    ON agent_messages (project_id, source_agent_id, target_agent_id)
    WHERE source_agent_id != '';

CREATE INDEX IF NOT EXISTS idx_agent_messages_source_created
    ON agent_messages (source_agent_id, created_at DESC);
