DROP INDEX IF EXISTS idx_code_deployments_unclaimed;

ALTER TABLE code_deployments
    DROP COLUMN IF EXISTS build_node_id,
    DROP COLUMN IF EXISTS build_node_claimed_at;
