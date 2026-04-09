-- Distributed build dispatch: track which orchestrator node claimed a deployment.
--
-- build_node_id:         UUID of the orchestrator worker that claimed this build.
--                        NULL means unclaimed and available for dispatch.
-- build_node_claimed_at: When the claim was made. Used to detect and recover
--                        stale claims from crashed workers (older than build_timeout*2).
--
-- The partial index below is used by ClaimBuildingDeployment to efficiently
-- find unclaimed work without scanning claimed or non-building rows.

ALTER TABLE code_deployments
    ADD COLUMN IF NOT EXISTS build_node_id         TEXT,
    ADD COLUMN IF NOT EXISTS build_node_claimed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_code_deployments_unclaimed
    ON code_deployments (created_at ASC)
    WHERE status = 'building' AND build_node_id IS NULL;
