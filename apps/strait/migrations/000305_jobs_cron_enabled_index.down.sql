-- Single statement so DROP INDEX CONCURRENTLY can run outside a transaction.
DROP INDEX CONCURRENTLY IF EXISTS idx_jobs_cron_enabled;
