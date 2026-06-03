package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"strait/internal/domain"

	"go.opentelemetry.io/otel"
)

var errPgQueNoMessages = errors.New("pgque: no messages available")

type pgQueActiveBatch struct {
	BatchID  int64
	Messages []pgQueMessage
	InFlight int
	Closing  bool
}

type pgQueBatchReservation struct {
	Batch      *pgQueActiveBatch
	Candidates []pgQueCandidate
	Invalid    []pgQueMessage
}

func (q *PgQueQueue) Dequeue(ctx context.Context) (*domain.JobRun, error) {
	runs, err := q.DequeueN(ctx, 1)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[0], nil
}

func (q *PgQueQueue) DequeueN(ctx context.Context, n int) ([]domain.JobRun, error) {
	return q.dequeueFromRoute(ctx, n, pgQueHTTPRouteKey, pgQueClaimFilter{
		ExecutionMode: domain.ExecutionModeHTTP,
	})
}

func (q *PgQueQueue) DequeueNByProject(ctx context.Context, n int, projectID string) ([]domain.JobRun, error) {
	return q.dequeueFromRoute(ctx, n, pgQueHTTPRouteKey, pgQueClaimFilter{
		ProjectID:     projectID,
		ExecutionMode: domain.ExecutionModeHTTP,
	})
}

func (q *PgQueQueue) DequeueNForWorkerQueues(ctx context.Context, n int, queues []domain.WorkerQueueRef) ([]domain.JobRun, error) {
	refs := normalizePgQueWorkerQueueRefs(queues)
	if n <= 0 || len(refs) == 0 {
		return nil, nil
	}
	routes, err := q.workerRouteKeys(ctx, refs)
	if err != nil {
		return nil, err
	}
	claimed := make([]domain.JobRun, 0, n)
	start := q.nextWorkerRouteStart(len(routes))
	for i := range routes {
		if len(claimed) >= n {
			break
		}
		routeKey := routes[(start+i)%len(routes)]
		batch, err := q.dequeueFromRoute(ctx, n-len(claimed), routeKey, pgQueClaimFilter{
			ExecutionMode: domain.ExecutionModeWorker,
			WorkerRefs:    refs,
		})
		if err != nil {
			return claimed, err
		}
		claimed = append(claimed, batch...)
	}
	return claimed, nil
}

func (q *PgQueQueue) dequeueFromRoute(ctx context.Context, n int, routeKey string, filter pgQueClaimFilter) ([]domain.JobRun, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "queue.PgQueDequeue")
	defer span.End()

	if n <= 0 {
		return nil, nil
	}
	queueName := pgQueQueueName(routeKey)
	state := q.routeState(routeKey)

	if err := q.ensureRouteCached(ctx, state, routeKey, queueName); err != nil {
		return nil, err
	}

	for attempt := 0; attempt < pgQueMaxAttempts; attempt++ {
		reservation, err := q.reserveFromActiveBatch(ctx, state, queueName, n)
		if err != nil {
			return nil, err
		}
		if reservation.Batch == nil {
			return nil, nil
		}

		for _, msg := range reservation.Invalid {
			_ = q.pgque(q.db).nack(ctx, msg, q.cfg.NackDelay, "invalid ready event")
		}
		if len(reservation.Candidates) == 0 {
			if err := q.finishBatchReservation(ctx, state, reservation.Batch, nil); err != nil {
				return nil, err
			}
			continue
		}

		runs, unclaimed, nackUnclaimed, err := q.claimReservedCandidates(ctx, reservation.Candidates, n, filter)
		returnCandidates := unclaimed
		if nackUnclaimed {
			for _, candidate := range unclaimed {
				_ = q.pgque(q.db).nack(ctx, candidate.Message, q.cfg.NackDelay, "not claimable")
			}
			returnCandidates = nil
		}
		if err != nil {
			returnCandidates = reservation.Candidates
		}
		if finishErr := q.finishBatchReservation(ctx, state, reservation.Batch, returnCandidates); finishErr != nil {
			return runs, finishErr
		}
		if err != nil {
			return nil, err
		}
		if len(runs) > 0 {
			for i := range runs {
				q.runWriter.recordClaimMetrics(ctx, &runs[i])
			}
			return runs, nil
		}
	}
	return nil, nil
}

func (q *PgQueQueue) reserveFromActiveBatch(ctx context.Context, state *pgQueRouteState, queueName string, limit int) (pgQueBatchReservation, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.activeBatch != nil && state.activeBatch.Closing {
		return pgQueBatchReservation{}, nil
	}
	if state.activeBatch != nil && len(state.activeBatch.Messages) == 0 && state.activeBatch.InFlight == 0 {
		return pgQueBatchReservation{Batch: state.activeBatch}, nil
	}
	if state.activeBatch == nil {
		q.maybeForceTick(ctx, state, queueName)
		_, err := q.activeBatchLocked(ctx, state, queueName)
		if errors.Is(err, errPgQueNoMessages) {
			return pgQueBatchReservation{}, nil
		}
		if err != nil {
			return pgQueBatchReservation{}, err
		}
	}
	batch := state.activeBatch
	if len(batch.Messages) == 0 {
		return pgQueBatchReservation{}, nil
	}

	candidates := make([]pgQueCandidate, 0, len(batch.Messages))
	invalid := make([]pgQueMessage, 0)
	removeIDs := make(map[int64]struct{})
	for i, msg := range batch.Messages {
		var event pgQueReadyEvent
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil || event.RunID == "" {
			invalid = append(invalid, msg)
			removeIDs[msg.ID] = struct{}{}
			continue
		}
		candidates = append(candidates, pgQueCandidate{Message: msg, Event: event, Order: i})
	}
	if len(candidates) > 0 {
		if err := q.refreshCandidateClaimState(ctx, candidates); err != nil {
			return pgQueBatchReservation{}, err
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].Event.Priority != candidates[j].Event.Priority {
				return candidates[i].Event.Priority > candidates[j].Event.Priority
			}
			return candidates[i].Order < candidates[j].Order
		})
		candidates = candidates[:min(len(candidates), limit)]
		for _, candidate := range candidates {
			removeIDs[candidate.Message.ID] = struct{}{}
		}
		batch.InFlight++
	}
	if len(removeIDs) > 0 {
		remaining := make([]pgQueMessage, 0, len(batch.Messages)-len(removeIDs))
		for _, msg := range batch.Messages {
			if _, ok := removeIDs[msg.ID]; ok {
				continue
			}
			remaining = append(remaining, msg)
		}
		batch.Messages = remaining
	}
	return pgQueBatchReservation{Batch: batch, Candidates: candidates, Invalid: invalid}, nil
}

func (q *PgQueQueue) refreshCandidateClaimState(ctx context.Context, candidates []pgQueCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.Event.RunID)
	}

	rows, err := q.db.Query(ctx, `
		WITH input AS (
			SELECT *
			FROM unnest($1::text[]) AS input(run_id)
		)
		SELECT input.run_id,
		       COALESCE(priority.priority, s.priority) AS priority,
		       s.job_max_concurrency IS NOT NULL
		           OR s.job_max_concurrency_per_key IS NOT NULL AS has_concurrency_limit
		FROM input
		JOIN job_run_state s ON s.run_id = input.run_id
		LEFT JOIN LATERAL (
		    SELECT e.priority
		    FROM job_run_priority_events e
		    WHERE e.run_id = input.run_id
		    ORDER BY e.id DESC
		    LIMIT 1
		) priority ON true`,
		ids,
	)
	if err != nil {
		return fmt.Errorf("pgque candidate priorities: %w", err)
	}
	defer rows.Close()

	stateByRunID := make(map[string]pgQueCandidateClaimState, len(candidates))
	for rows.Next() {
		var state pgQueCandidateClaimState
		if err := rows.Scan(
			&state.runID,
			&state.priority,
			&state.hasConcurrencyLimit,
		); err != nil {
			return fmt.Errorf("pgque candidate claim state scan: %w", err)
		}
		stateByRunID[state.runID] = state
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("pgque candidate claim state rows: %w", err)
	}
	for i := range candidates {
		state, ok := stateByRunID[candidates[i].Event.RunID]
		if !ok {
			continue
		}
		candidates[i].Event.Priority = state.priority
		candidates[i].HasConcurrencyLimit = state.hasConcurrencyLimit
	}
	return nil
}

type pgQueCandidateClaimState struct {
	runID               string
	priority            int
	hasConcurrencyLimit bool
}

func (q *PgQueQueue) finishBatchReservation(ctx context.Context, state *pgQueRouteState, batch *pgQueActiveBatch, returnCandidates []pgQueCandidate) error {
	if batch == nil {
		return nil
	}
	if !q.closeBatchIfDrained(state, batch, returnCandidates) {
		return nil
	}
	if err := q.pgque(q.db).ack(ctx, batch.BatchID); err != nil {
		q.reopenBatchAfterAckFailure(state, batch)
		return err
	}
	q.clearAckedBatch(state, batch)
	return nil
}

func (q *PgQueQueue) closeBatchIfDrained(state *pgQueRouteState, batch *pgQueActiveBatch, returnCandidates []pgQueCandidate) bool {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.activeBatch == batch && !batch.Closing {
		for _, candidate := range returnCandidates {
			batch.Messages = append(batch.Messages, candidate.Message)
		}
		if batch.InFlight > 0 {
			batch.InFlight--
		}
		if len(batch.Messages) == 0 && batch.InFlight == 0 {
			batch.Closing = true
		}
	}
	return state.activeBatch == batch && batch.Closing
}

func (q *PgQueQueue) reopenBatchAfterAckFailure(state *pgQueRouteState, batch *pgQueActiveBatch) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.activeBatch == batch {
		batch.Closing = false
	}
}

func (q *PgQueQueue) clearAckedBatch(state *pgQueRouteState, batch *pgQueActiveBatch) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.activeBatch == batch {
		state.activeBatch = nil
	}
}

// activeBatchLocked requires state.mu to be held by the caller. PgQue batches
// are acked as a unit, so local reservations must mutate state.activeBatch
// synchronously with receive/ack bookkeeping.
func (q *PgQueQueue) activeBatchLocked(ctx context.Context, state *pgQueRouteState, queueName string) (*pgQueActiveBatch, error) {
	if batch := state.activeBatch; batch != nil && (len(batch.Messages) > 0 || batch.InFlight > 0 || batch.Closing) {
		return batch, nil
	}
	messages, err := q.pgque(q.db).receive(ctx, queueName, pgQueReceiveAll)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, errPgQueNoMessages
	}
	batch := &pgQueActiveBatch{BatchID: messages[0].BatchID, Messages: messages}
	state.activeBatch = batch
	return batch, nil
}
