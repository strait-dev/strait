package scheduler

import "strait/internal/domain"

// R4 Phase 3: default watched queries for the plan drift monitor.
//
// Each entry pairs a human-readable name with the SQL text that the
// monitor will EXPLAIN daily to capture its baseline plan. The SQL
// uses safe dummy values for parameterised positions ($1, $2, ...) so
// EXPLAIN can parse and plan them without side effects.

// DefaultWatchedQueries returns the hot-path queries the plan drift
// monitor should baseline. Every dequeue variant and the major
// scheduler maintenance queries are included.
func DefaultWatchedQueries() []WatchedQuery {
	return []WatchedQuery{
		{
			Name: "DequeueN",
			SQL: `SELECT id FROM job_runs jr
				JOIN jobs j ON j.id = jr.job_id
				WHERE jr.status = '` + string(domain.StatusQueued) + `'
				  AND j.enabled = true AND NOT j.paused
				ORDER BY jr.priority DESC, jr.created_at ASC
				LIMIT 10`,
		},
		{
			Name: "DequeueNDenormalized",
			SQL: `SELECT id FROM job_runs jr
				LEFT JOIN job_active_counts jac ON jac.job_id = jr.job_id AND jac.concurrency_key = ''
				WHERE jr.status = '` + string(domain.StatusQueued) + `'
				ORDER BY jr.priority DESC, jr.created_at ASC
				LIMIT 10`,
		},
		{
			Name: "DequeueNFullyDenormalized",
			SQL: `SELECT id FROM job_runs jr
				LEFT JOIN job_active_counts jac ON jac.job_id = jr.job_id AND jac.concurrency_key = ''
				WHERE jr.status = '` + string(domain.StatusQueued) + `'
				  AND COALESCE(jr.job_enabled, true) = true
				  AND COALESCE(jr.job_paused, false) = false
				ORDER BY jr.priority DESC, jr.created_at ASC
				LIMIT 10`,
		},
		{
			Name: "HeartbeatGC",
			SQL: `SELECT h.run_id FROM job_run_heartbeats h
				LEFT JOIN job_runs r ON r.id = h.run_id
				WHERE r.id IS NULL OR r.status <> 'executing'
				LIMIT 500`,
		},
		{
			Name: "DLQAgeOutMask",
			SQL: `SELECT id FROM job_runs
				WHERE status = 'dead_letter'
				  AND visible_until IS NULL
				  AND finished_at IS NOT NULL
				  AND finished_at < NOW() - INTERVAL '30 days'
				ORDER BY finished_at ASC
				LIMIT 100`,
		},
		{
			Name: "OldestQueuedAge",
			SQL: `SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at))), 0)
				FROM job_runs
				WHERE status = 'queued'`,
		},
	}
}
