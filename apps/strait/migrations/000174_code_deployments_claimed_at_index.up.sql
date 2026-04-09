-- Add index on build_node_claimed_at so ReleaseStaleClaimedDeployments can
-- efficiently find rows to reset without a full table scan.
-- Without this index the query does a seq scan on every GC cycle.
CREATE INDEX IF NOT EXISTS idx_code_deployments_claimed_at
    ON code_deployments (build_node_claimed_at)
    WHERE build_node_claimed_at IS NOT NULL;
