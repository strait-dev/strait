-- Add index on job_runs.continuation_of for lineage queries.
-- Used by ListRunLineage recursive lookups.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_runs_continuation_of
    ON job_runs (continuation_of)
    WHERE continuation_of IS NOT NULL;

-- Add index on pricing_catalog for cost lookups.
-- Used by lookupPricing in CreateRunUsage hot path.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_pricing_catalog_provider_model_active
    ON pricing_catalog (provider, model, active, effective_from DESC);

-- Add index on webhook_deliveries.job_id for ListWebhookDeliveries JOIN.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_webhook_deliveries_job_id
    ON webhook_deliveries (job_id);
