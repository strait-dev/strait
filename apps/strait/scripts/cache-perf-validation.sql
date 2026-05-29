-- Cache-overhaul perf validation.
--
-- Run this before and after enabling the cache overhaul against the same load
-- profile. For a clean window:
--
--   SELECT pg_stat_statements_reset();
--   -- run the workload for the same duration on both builds
--   \i apps/strait/scripts/cache-perf-validation.sql
--
-- The "after" run should show lower calls for the hot reads listed here.

WITH targets(label, pattern) AS (
	VALUES
		('api_key_by_hash', '%FROM api_keys%WHERE key_hash = $1%'),
		('get_job', '%FROM jobs%WHERE id = $1%'),
		('job_health_stats', '%FROM job_runs%COUNT(*) FILTER%'),
		('job_dependencies', '%FROM job_dependencies%depends_on_job_id%'),
		('workflow_step_listing', '%FROM workflow_steps%workflow_id = $1%version = $2%'),
		('workflow_run_by_id', '%FROM workflow_runs%WHERE id = $1%')
)
SELECT
	targets.label,
	COALESCE(SUM(pg_stat_statements.calls), 0)::bigint AS calls,
	ROUND(COALESCE(SUM(pg_stat_statements.total_exec_time), 0)::numeric, 2) AS total_exec_ms,
	ROUND(COALESCE(SUM(pg_stat_statements.mean_exec_time * pg_stat_statements.calls) / NULLIF(SUM(pg_stat_statements.calls), 0), 0)::numeric, 3) AS weighted_mean_exec_ms,
	COUNT(pg_stat_statements.queryid) AS matched_statements
FROM targets
LEFT JOIN pg_stat_statements
	ON pg_stat_statements.query ILIKE targets.pattern
GROUP BY targets.label
ORDER BY targets.label;
