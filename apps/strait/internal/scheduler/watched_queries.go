package scheduler

import "strait/internal/domain"

// Default watched queries for the plan drift monitor.
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
			SQL: `SELECT jr.id FROM job_run_state s
				JOIN job_runs jr ON jr.id = s.run_id
				JOIN jobs j ON j.id = jr.job_id
				WHERE s.status = '` + string(domain.StatusQueued) + `'
				  AND j.enabled = true AND NOT j.paused
				ORDER BY s.priority DESC, jr.created_at ASC
				LIMIT 10`,
		},
		{
			Name: "PgQueClaimCandidates",
			SQL: `SELECT jr.id FROM job_run_state s
				JOIN job_runs jr ON jr.id = s.run_id
				LEFT JOIN job_active_counts jac ON jac.job_id = s.job_id AND jac.concurrency_key = ''
				LEFT JOIN LATERAL (
					SELECT e.priority
					FROM job_run_priority_events e
					WHERE e.run_id = s.run_id
					ORDER BY e.id DESC
					LIMIT 1
				) priority ON true
				WHERE s.status = '` + string(domain.StatusQueued) + `'
				  AND s.job_enabled = true
				  AND s.job_paused = false
				ORDER BY COALESCE(priority.priority, s.priority) DESC, jr.created_at ASC
				LIMIT 10`,
		},
		{
			Name: "HeartbeatGC",
			SQL: `SELECT h.run_id FROM job_run_heartbeats h
				LEFT JOIN job_run_read_state s ON s.run_id = h.run_id
				WHERE h.cleared = FALSE
				  AND NOT EXISTS (
				    SELECT 1 FROM job_run_heartbeats newer
				    WHERE newer.run_id = h.run_id AND newer.id > h.id
				  )
				  AND (s.run_id IS NULL OR s.status <> 'executing')
				LIMIT 500`,
		},
		{
			Name: "DLQAgeOutMask",
			SQL: `SELECT s.run_id FROM job_run_read_state s
				JOIN job_runs jr ON jr.id = s.run_id
				WHERE s.status = 'dead_letter'
				  AND jr.visible_until IS NULL
				  AND s.finished_at IS NOT NULL
				  AND s.finished_at < NOW() - INTERVAL '30 days'
				ORDER BY s.finished_at ASC
				LIMIT 100`,
		},
		{
			Name: "OldestQueuedAge",
			SQL: `SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at))), 0)
				FROM job_runs jr
				JOIN job_run_read_state s ON s.run_id = jr.id
				WHERE s.status = 'queued'`,
		},
	}
}
