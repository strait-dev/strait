package queue

import (
	"context"
	"fmt"

	"strait/internal/dbscan"
	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
)

type pgQueCandidate struct {
	Message             pgQueMessage
	Event               pgQueReadyEvent
	Order               int
	HasConcurrencyLimit bool
}

type pgQueClaimFilter struct {
	ProjectID     string
	ExecutionMode domain.ExecutionMode
	WorkerRefs    []domain.WorkerQueueRef
	workerRefArgs pgQueWorkerRefArgs
}

type pgQueClaimSelection struct {
	Candidates          []pgQueCandidate
	RunIDs              []string
	Generations         []int64
	HasConcurrencyLimit bool
}

type pgQueClaimSelectionBuffer struct {
	runIDs      [pgQueSmallCandidateSetLimit]string
	generations [pgQueSmallCandidateSetLimit]int64
}

type pgQueClaimRunRequest struct {
	RunIDs              []string
	Generations         []int64
	Limit               int
	Filter              pgQueClaimFilter
	HasConcurrencyLimit bool
}

const pgQueClaimDequeueColumns = `u.run_id, u.job_id, u.project_id, u.status, u.attempt, u.payload, u.result, u.metadata, u.error, u.error_class,
		          u.triggered_by, u.scheduled_at, u.started_at, u.finished_at, u.heartbeat_at,
		          u.next_retry_at, u.expires_at, u.parent_run_id, u.priority, u.idempotency_key, u.job_version, u.created_at, u.workflow_step_run_id, u.execution_trace, u.debug_mode, u.continuation_of, u.lineage_depth, u.tags, u.job_version_id, u.created_by, u.batch_id, u.concurrency_key, u.execution_mode, u.is_rollback, u.replayed_run_id`

func (q *PgQueQueue) claimReservedCandidates(ctx context.Context, candidates []pgQueCandidate, limit int, filter pgQueClaimFilter) ([]domain.JobRun, []pgQueCandidate, bool, error) {
	if len(candidates) == 0 {
		return nil, nil, false, nil
	}
	var selectionBuffer pgQueClaimSelectionBuffer
	selection := selectPgQueClaimCandidates(candidates, limit, &selectionBuffer)

	runs, err := q.claimRuns(ctx, pgQueClaimRunRequest{
		RunIDs:              selection.RunIDs,
		Generations:         selection.Generations,
		Limit:               limit,
		Filter:              filter,
		HasConcurrencyLimit: selection.HasConcurrencyLimit,
	})
	if err != nil {
		return nil, nil, false, err
	}
	if len(runs) == 0 {
		return nil, selection.Candidates, true, nil
	}

	unclaimed := unclaimedReservedCandidates(candidates, runs)
	return runs, unclaimed, false, nil
}

func selectPgQueClaimCandidates(
	candidates []pgQueCandidate,
	limit int,
	buffer *pgQueClaimSelectionBuffer,
) pgQueClaimSelection {
	selected := candidates[:min(len(candidates), limit)]
	var ids []string
	var generations []int64
	if buffer != nil && len(selected) <= pgQueSmallCandidateSetLimit {
		ids = buffer.runIDs[:len(selected)]
		generations = buffer.generations[:len(selected)]
	} else {
		ids = make([]string, len(selected))
		generations = make([]int64, len(selected))
	}
	hasConcurrencyLimit := false
	for i, candidate := range selected {
		ids[i] = candidate.Event.RunID
		generations[i] = candidate.Event.Generation
		if candidate.HasConcurrencyLimit {
			hasConcurrencyLimit = true
		}
	}

	return pgQueClaimSelection{
		Candidates:          selected,
		RunIDs:              ids,
		Generations:         generations,
		HasConcurrencyLimit: hasConcurrencyLimit,
	}
}

func unclaimedReservedCandidates(candidates []pgQueCandidate, runs []domain.JobRun) []pgQueCandidate {
	if len(runs) == 0 {
		return candidates
	}
	if len(runs) == len(candidates) {
		clear(candidates)
		return nil
	}

	if len(runs) <= pgQueSmallCandidateSetLimit {
		return compactUnclaimedReservedCandidates(candidates, func(runID string) bool {
			return pgQueRunClaimedLinear(runs, runID)
		})
	}

	claimed := make(map[string]struct{}, len(runs))
	for _, run := range runs {
		claimed[run.ID] = struct{}{}
	}

	return compactUnclaimedReservedCandidates(candidates, func(runID string) bool {
		_, ok := claimed[runID]
		return ok
	})
}

func compactUnclaimedReservedCandidates(candidates []pgQueCandidate, claimed func(runID string) bool) []pgQueCandidate {
	write := 0
	for _, candidate := range candidates {
		if claimed(candidate.Event.RunID) {
			continue
		}
		candidates[write] = candidate
		write++
	}
	clear(candidates[write:])
	return candidates[:write]
}

func pgQueRunClaimedLinear(runs []domain.JobRun, runID string) bool {
	for _, run := range runs {
		if run.ID == runID {
			return true
		}
	}
	return false
}

func (q *PgQueQueue) claimRuns(ctx context.Context, req pgQueClaimRunRequest) ([]domain.JobRun, error) {
	if len(req.RunIDs) == 0 {
		return nil, nil
	}
	if len(req.RunIDs) != len(req.Generations) {
		return nil, fmt.Errorf("pgque claim runs: mismatched id/generation counts")
	}

	if req.HasConcurrencyLimit {
		return q.claimRunsWithConcurrency(ctx, req)
	}
	return q.claimRunsUnconstrained(ctx, req)
}

func (q *PgQueQueue) claimRunsUnconstrained(ctx context.Context, req pgQueClaimRunRequest) ([]domain.JobRun, error) {
	workerArgs := req.Filter.workerArgs()
	rows, err := q.db.Query(ctx, fmt.Sprintf(`
		WITH input AS (
			SELECT *
			FROM unnest($1::text[], $2::bigint[]) WITH ORDINALITY AS u(id, generation, ord)
		),
		raw_candidates AS (
			SELECT s.run_id,
			       input.ord,
			       input.generation,
			       s.job_id,
			       s.project_id,
			       s.concurrency_key,
			       s.job_max_concurrency,
			       s.job_max_concurrency_per_key,
			       s.ready_attempt,
			       s.ready_reason,
			       COALESCE(s.promoted_priority, s.priority) AS claim_priority,
			       jr.created_at AS claim_created_at,
			       jr.payload,
			       jr.result,
			       jr.metadata,
			       jr.error,
			       jr.error_class,
			       jr.triggered_by,
			       jr.parent_run_id,
			       jr.idempotency_key,
			       jr.job_version,
			       jr.created_at,
			       jr.workflow_step_run_id,
			       jr.execution_trace,
			       jr.debug_mode,
			       jr.continuation_of,
			       jr.lineage_depth,
			       jr.tags,
			       jr.job_version_id,
			       jr.created_by,
			       jr.batch_id,
			       jr.is_rollback,
			       jr.replayed_run_id
			FROM input
			JOIN LATERAL (
				SELECT s.*,
				       priority.priority AS promoted_priority,
				       ready.attempt AS ready_attempt,
				       ready.reason AS ready_reason
				FROM job_run_state s
				LEFT JOIN LATERAL (
				    SELECT e.priority
				    FROM job_run_priority_events e
				    WHERE e.run_id = s.run_id
				    ORDER BY e.id DESC
				    LIMIT 1
				) priority ON true
				LEFT JOIN LATERAL (
				    SELECT e.attempt, e.reason
				    FROM job_run_ready_events e
				    WHERE e.run_id = s.run_id
				      AND e.ready_generation = s.ready_generation
				    ORDER BY e.id DESC
				    LIMIT 1
				) ready ON true
				WHERE s.run_id = input.id
				  AND (
				      s.status = $4
				      OR (s.status = 'delayed' AND ready.reason = 'delayed_due')
				      OR (s.status = 'delayed' AND ready.reason = 'worker_recovered')
				      OR (s.status = 'paused' AND ready.reason = 'paused_resume')
				  )
				  AND s.ready_generation = input.generation
				  AND s.execution_mode = $6
				  AND ($7::text = '' OR s.project_id = $7)
				  AND (
				      $6 <> 'worker'
				      OR EXISTS (
				          SELECT 1
				          FROM unnest($8::text[], $9::text[], $10::text[]) AS wq(project_id, queue_name, environment_id)
				          WHERE wq.project_id = s.project_id
				            AND wq.queue_name = s.queue_name
				            AND (wq.environment_id = '' OR s.environment_id = wq.environment_id)
				      )
				  )
				  AND COALESCE(s.job_enabled, true) = true
				  AND COALESCE(s.job_paused, false) = false
				  AND (
				      s.scheduled_at IS NULL
				      OR s.scheduled_at <= NOW()
				      OR ready.reason = 'worker_recovered'
				  )
				  AND (
				      s.next_retry_at IS NULL
				      OR s.next_retry_at <= NOW()
				      OR ready.reason = 'retry_ready'
				      OR ready.reason = 'worker_recovered'
				  )
				  AND NOT strait_run_retry_blocked(s.run_id)
				  AND NOT EXISTS (
				      SELECT 1
				      FROM job_run_active_claims c
				      WHERE c.run_id = s.run_id
				        AND c.ready_generation = s.ready_generation
				  )
				FOR UPDATE OF s SKIP LOCKED
			) s ON true
			JOIN job_runs jr ON jr.id = s.run_id
			ORDER BY s.priority DESC, jr.created_at ASC, input.ord
		),
		candidates AS (
			SELECT *
			FROM raw_candidates
			ORDER BY claim_priority DESC, claim_created_at ASC, ord
			LIMIT $3
		),
		inserted_claims AS (
			INSERT INTO job_run_active_claims (
				run_id,
				ready_generation,
				attempt,
				started_at
			)
			SELECT
				s.run_id,
				s.ready_generation,
				COALESCE(candidates.ready_attempt, s.attempt),
				NOW()
			FROM job_run_state s
			JOIN candidates ON candidates.run_id = s.run_id
			WHERE (
			      s.status IN ($4, 'delayed')
			      OR (
			          s.status = 'paused'
			          AND candidates.ready_reason = 'paused_resume'
			      )
			  )
			  AND s.ready_generation = candidates.generation
			  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
			ON CONFLICT (run_id, ready_generation) DO NOTHING
			RETURNING run_id, ready_generation, attempt, started_at
		),
		claimed_state AS (
			SELECT s.run_id,
			       candidates.job_id,
			       candidates.project_id,
			       $5::text AS status,
			       i.attempt,
			       candidates.payload,
			       candidates.result,
			       candidates.metadata,
			       candidates.error,
			       candidates.error_class,
			       candidates.triggered_by,
			       s.scheduled_at,
			       i.started_at,
			       CASE WHEN candidates.ready_reason = 'retry_ready' THEN NULL::timestamptz ELSE s.finished_at END AS finished_at,
			       CASE WHEN candidates.ready_reason = 'retry_ready' THEN NULL::timestamptz ELSE s.heartbeat_at END AS heartbeat_at,
			       CASE WHEN candidates.ready_reason = 'retry_ready' THEN NULL::timestamptz ELSE s.next_retry_at END AS next_retry_at,
			       s.expires_at,
			       candidates.parent_run_id,
			       candidates.claim_priority AS priority,
			       candidates.idempotency_key,
			       candidates.job_version,
			       candidates.created_at,
			       candidates.workflow_step_run_id,
			       candidates.execution_trace,
			       candidates.debug_mode,
			       candidates.continuation_of,
			       candidates.lineage_depth,
			       candidates.tags,
			       candidates.job_version_id,
			       candidates.created_by,
			       candidates.batch_id,
			       s.concurrency_key,
			       s.execution_mode,
			       candidates.is_rollback,
			       candidates.replayed_run_id,
			       candidates.claim_priority,
			       candidates.claim_created_at,
			       candidates.ord
			FROM inserted_claims i
			JOIN job_run_state s ON s.run_id = i.run_id AND s.ready_generation = i.ready_generation
			JOIN candidates ON candidates.run_id = i.run_id
		)
		SELECT %s
		FROM claimed_state u
		ORDER BY u.claim_priority DESC, u.claim_created_at ASC, u.ord`,
		pgQueClaimDequeueColumns,
	),
		req.RunIDs,
		req.Generations,
		req.Limit,
		domain.StatusQueued,
		domain.StatusExecuting,
		req.Filter.ExecutionMode,
		req.Filter.ProjectID,
		workerArgs.ProjectIDs,
		workerArgs.QueueNames,
		workerArgs.EnvironmentIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("pgque claim unconstrained runs: %w", err)
	}
	return scanPgQueClaimedRuns(rows, req.Limit, "pgque claim unconstrained")
}

func (q *PgQueQueue) claimRunsWithConcurrency(ctx context.Context, req pgQueClaimRunRequest) ([]domain.JobRun, error) {
	workerArgs := req.Filter.workerArgs()
	rows, err := q.db.Query(ctx, fmt.Sprintf(`
		WITH input AS (
			SELECT *
			FROM unnest($1::text[], $2::bigint[]) WITH ORDINALITY AS u(id, generation, ord)
		),
		raw_candidates AS (
			SELECT s.run_id,
			       input.ord,
			       input.generation,
			       s.job_id,
			       s.project_id,
			       s.concurrency_key,
			       s.job_max_concurrency,
			       s.job_max_concurrency_per_key,
			       s.ready_attempt,
			       s.ready_reason,
			       COALESCE(s.promoted_priority, s.priority) AS claim_priority,
			       jr.created_at AS claim_created_at,
			       jr.payload,
			       jr.result,
			       jr.metadata,
			       jr.error,
			       jr.error_class,
			       jr.triggered_by,
			       jr.parent_run_id,
			       jr.idempotency_key,
			       jr.job_version,
			       jr.created_at,
			       jr.workflow_step_run_id,
			       jr.execution_trace,
			       jr.debug_mode,
			       jr.continuation_of,
			       jr.lineage_depth,
			       jr.tags,
			       jr.job_version_id,
			       jr.created_by,
			       jr.batch_id,
			       jr.is_rollback,
			       jr.replayed_run_id
			FROM input
			JOIN LATERAL (
				SELECT s.*,
				       priority.priority AS promoted_priority,
				       ready.attempt AS ready_attempt,
				       ready.reason AS ready_reason
				FROM job_run_state s
				LEFT JOIN LATERAL (
				    SELECT e.priority
				    FROM job_run_priority_events e
				    WHERE e.run_id = s.run_id
				    ORDER BY e.id DESC
				    LIMIT 1
				) priority ON true
				LEFT JOIN LATERAL (
				    SELECT e.attempt, e.reason
				    FROM job_run_ready_events e
				    WHERE e.run_id = s.run_id
				      AND e.ready_generation = s.ready_generation
				    ORDER BY e.id DESC
				    LIMIT 1
				) ready ON true
				WHERE s.run_id = input.id
				  AND (
				      s.status = $4
				      OR (s.status = 'delayed' AND ready.reason = 'delayed_due')
				      OR (s.status = 'delayed' AND ready.reason = 'worker_recovered')
				      OR (s.status = 'paused' AND ready.reason = 'paused_resume')
				  )
				  AND s.ready_generation = input.generation
				  AND s.execution_mode = $6
				  AND ($7::text = '' OR s.project_id = $7)
				  AND (
				      $6 <> 'worker'
				      OR EXISTS (
				          SELECT 1
				          FROM unnest($8::text[], $9::text[], $10::text[]) AS wq(project_id, queue_name, environment_id)
				          WHERE wq.project_id = s.project_id
				            AND wq.queue_name = s.queue_name
				            AND (wq.environment_id = '' OR s.environment_id = wq.environment_id)
				      )
				  )
				  AND COALESCE(s.job_enabled, true) = true
				  AND COALESCE(s.job_paused, false) = false
				  AND (
				      s.scheduled_at IS NULL
				      OR s.scheduled_at <= NOW()
				      OR ready.reason = 'worker_recovered'
				  )
				  AND (
				      s.next_retry_at IS NULL
				      OR s.next_retry_at <= NOW()
				      OR ready.reason = 'retry_ready'
				      OR ready.reason = 'worker_recovered'
				  )
				  AND NOT strait_run_retry_blocked(s.run_id)
				  AND NOT EXISTS (
				      SELECT 1
				      FROM job_run_active_claims c
				      WHERE c.run_id = s.run_id
				        AND c.ready_generation = s.ready_generation
				  )
				FOR UPDATE OF s SKIP LOCKED
			) s ON true
			JOIN job_runs jr ON jr.id = s.run_id
			ORDER BY s.priority DESC, jr.created_at ASC, input.ord
		),
		limited_jobs AS MATERIALIZED (
			SELECT DISTINCT raw_candidates.job_id
			FROM raw_candidates
			WHERE job_max_concurrency IS NOT NULL
			   OR job_max_concurrency_per_key IS NOT NULL
			ORDER BY raw_candidates.job_id
		),
		job_locks AS MATERIALIZED (
			SELECT pg_advisory_xact_lock(hashtextextended(limited_jobs.job_id, 0)) AS locked
			FROM limited_jobs
		),
		lock_barrier AS MATERIALIZED (
			SELECT COUNT(*) AS locked_jobs FROM job_locks
		),
		active_key_counts AS MATERIALIZED (
			SELECT
				active.job_id,
				COALESCE(active.concurrency_key, '') AS concurrency_key,
				COUNT(*)::int AS count
			FROM job_run_state active
			JOIN limited_jobs limited ON limited.job_id = active.job_id
			JOIN job_run_active_claims claim
			  ON claim.run_id = active.run_id
			 AND claim.ready_generation = active.ready_generation
			LEFT JOIN job_run_terminal_state terminal ON terminal.run_id = active.run_id
			CROSS JOIN lock_barrier
			WHERE (
			      active.status IN ($4, 'delayed')
			      OR (
			          active.status = 'paused'
			          AND EXISTS (
			              SELECT 1
			              FROM job_run_ready_events ready
			              WHERE ready.run_id = active.run_id
			                AND ready.ready_generation = active.ready_generation
			                AND ready.reason = 'paused_resume'
			          )
			      )
			  )
			  AND terminal.run_id IS NULL
			GROUP BY active.job_id, COALESCE(active.concurrency_key, '')
		),
		active_job_counts AS MATERIALIZED (
			SELECT active_key_counts.job_id, SUM(active_key_counts.count)::int AS count
			FROM active_key_counts
			GROUP BY active_key_counts.job_id
		),
		ranked_candidates AS (
			SELECT raw_candidates.*,
			       COALESCE(active_job_counts.count, 0) AS active_count,
			       COALESCE(active_key_counts.count, 0) AS key_active_count,
			       ROW_NUMBER() OVER (PARTITION BY raw_candidates.job_id ORDER BY claim_priority DESC, claim_created_at ASC, ord) AS job_rank,
			       ROW_NUMBER() OVER (PARTITION BY raw_candidates.job_id, raw_candidates.concurrency_key ORDER BY claim_priority DESC, claim_created_at ASC, ord) AS key_rank
			FROM raw_candidates
			CROSS JOIN lock_barrier
			LEFT JOIN active_job_counts ON active_job_counts.job_id = raw_candidates.job_id
			LEFT JOIN active_key_counts
			  ON active_key_counts.job_id = raw_candidates.job_id
			 AND active_key_counts.concurrency_key = COALESCE(raw_candidates.concurrency_key, '')
		),
		candidates AS (
			SELECT *
			FROM ranked_candidates
			WHERE (job_max_concurrency IS NULL OR job_rank <= GREATEST(job_max_concurrency - active_count, 0))
			  AND (
			      job_max_concurrency_per_key IS NULL
			      OR concurrency_key = ''
			      OR key_rank <= GREATEST(job_max_concurrency_per_key - key_active_count, 0)
			  )
			ORDER BY claim_priority DESC, claim_created_at ASC, ord
			LIMIT $3
		),
		inserted_claims AS (
			INSERT INTO job_run_active_claims (
				run_id,
				ready_generation,
				attempt,
				started_at
			)
			SELECT
				s.run_id,
				s.ready_generation,
				COALESCE(candidates.ready_attempt, s.attempt),
				NOW()
			FROM job_run_state s
			JOIN candidates ON candidates.run_id = s.run_id
			WHERE (
			      s.status IN ($4, 'delayed')
			      OR (
			          s.status = 'paused'
			          AND candidates.ready_reason = 'paused_resume'
			      )
			  )
			  AND s.ready_generation = candidates.generation
			  AND NOT EXISTS (SELECT 1 FROM job_run_terminal_state t WHERE t.run_id = s.run_id)
			ON CONFLICT (run_id, ready_generation) DO NOTHING
			RETURNING run_id, ready_generation, attempt, started_at
		),
		claimed_state AS (
			SELECT s.run_id,
			       candidates.job_id,
			       candidates.project_id,
			       $5::text AS status,
			       i.attempt,
			       candidates.payload,
			       candidates.result,
			       candidates.metadata,
			       candidates.error,
			       candidates.error_class,
			       candidates.triggered_by,
			       s.scheduled_at,
			       i.started_at,
			       CASE WHEN candidates.ready_reason = 'retry_ready' THEN NULL::timestamptz ELSE s.finished_at END AS finished_at,
			       CASE WHEN candidates.ready_reason = 'retry_ready' THEN NULL::timestamptz ELSE s.heartbeat_at END AS heartbeat_at,
			       CASE WHEN candidates.ready_reason = 'retry_ready' THEN NULL::timestamptz ELSE s.next_retry_at END AS next_retry_at,
			       s.expires_at,
			       candidates.parent_run_id,
			       candidates.claim_priority AS priority,
			       candidates.idempotency_key,
			       candidates.job_version,
			       candidates.created_at,
			       candidates.workflow_step_run_id,
			       candidates.execution_trace,
			       candidates.debug_mode,
			       candidates.continuation_of,
			       candidates.lineage_depth,
			       candidates.tags,
			       candidates.job_version_id,
			       candidates.created_by,
			       candidates.batch_id,
			       s.concurrency_key,
			       s.execution_mode,
			       candidates.is_rollback,
			       candidates.replayed_run_id,
			       candidates.claim_priority,
			       candidates.claim_created_at,
			       candidates.ord
			FROM inserted_claims i
			JOIN job_run_state s ON s.run_id = i.run_id AND s.ready_generation = i.ready_generation
			JOIN candidates ON candidates.run_id = i.run_id
		)
		SELECT %s
		FROM claimed_state u
		ORDER BY u.claim_priority DESC, u.claim_created_at ASC, u.ord`,
		pgQueClaimDequeueColumns,
	),
		req.RunIDs,
		req.Generations,
		req.Limit,
		domain.StatusQueued,
		domain.StatusExecuting,
		req.Filter.ExecutionMode,
		req.Filter.ProjectID,
		workerArgs.ProjectIDs,
		workerArgs.QueueNames,
		workerArgs.EnvironmentIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("pgque claim runs: %w", err)
	}
	return scanPgQueClaimedRuns(rows, req.Limit, "pgque claim")
}

func scanPgQueClaimedRuns(rows pgx.Rows, limit int, label string) ([]domain.JobRun, error) {
	defer rows.Close()

	runs := make([]domain.JobRun, 0, limit)
	for rows.Next() {
		run, err := dbscan.ScanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("%s scan: %w", label, err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s rows: %w", label, err)
	}
	return runs, nil
}
