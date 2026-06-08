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

type pgQueWorkerRouteScanner func(routeKey string, remaining int) ([]domain.JobRun, error)

const (
	pgQueSmallCandidateSetLimit = 8
	pgQueSmallRemoveSetLimit    = 8
)

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
	filter := pgQueClaimFilter{
		ExecutionMode: domain.ExecutionModeWorker,
		WorkerRefs:    refs,
		workerRefArgs: workerQueueRefArgsFromNormalized(refs),
	}
	return q.scanWorkerRoutes(routes, n, func(routeKey string, remaining int) ([]domain.JobRun, error) {
		return q.dequeueFromRoute(ctx, remaining, routeKey, filter)
	})
}

func (q *PgQueQueue) scanWorkerRoutes(
	routes []string,
	n int,
	scan pgQueWorkerRouteScanner,
) ([]domain.JobRun, error) {
	if n <= 0 || len(routes) == 0 {
		return nil, nil
	}
	if len(routes) == 1 {
		return scan(routes[0], n)
	}
	claimed := make([]domain.JobRun, 0, n)
	start := q.nextWorkerRouteStart(len(routes))
	for i := range routes {
		if len(claimed) >= n {
			break
		}
		routeKey := routes[(start+i)%len(routes)]
		batch, err := scan(routeKey, n-len(claimed))
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
	state := q.routeState(routeKey)
	queueName := state.queueName

	if err := q.ensureRouteCached(ctx, state, routeKey, queueName); err != nil {
		return nil, err
	}

	claimed := make([]domain.JobRun, 0, n)
	emptyAttempts := 0
	for batchScan := 0; batchScan < pgQueMaxDequeueFillBatches && len(claimed) < n && emptyAttempts < pgQueMaxAttempts; batchScan++ {
		remaining := n - len(claimed)
		reservation, err := q.reserveFromActiveBatch(ctx, state, queueName, remaining)
		if err != nil {
			if len(claimed) > 0 {
				q.logBackgroundError(ctx, "dequeue_fill", "pgque dequeue fill stopped after partial claim", err)
				return claimed, nil
			}
			return nil, err
		}
		if reservation.Batch == nil {
			return claimed, nil
		}

		for _, msg := range reservation.Invalid {
			q.nackReservedMessage(ctx, msg, "invalid ready event")
		}
		if len(reservation.Candidates) == 0 {
			if err := q.finishBatchReservation(ctx, state, reservation.Batch, nil); err != nil {
				if len(claimed) > 0 {
					q.logBackgroundError(ctx, "dequeue_fill", "pgque empty batch finish failed after partial claim", err)
					return claimed, nil
				}
				return nil, err
			}
			emptyAttempts++
			continue
		}

		runs, unclaimed, nackUnclaimed, err := q.claimReservedCandidates(ctx, reservation.Candidates, remaining, filter)
		returnCandidates := unclaimed
		if nackUnclaimed {
			for _, candidate := range unclaimed {
				q.nackReservedMessage(ctx, candidate.Message, "not claimable")
			}
			returnCandidates = nil
		}
		if err != nil {
			returnCandidates = reservation.Candidates
		}
		if finishErr := q.finishBatchReservation(ctx, state, reservation.Batch, returnCandidates); finishErr != nil {
			if len(runs) > 0 {
				for i := range runs {
					q.runWriter.recordClaimMetrics(ctx, &runs[i])
				}
				claimed = append(claimed, runs...)
			}
			if len(claimed) > 0 {
				q.logBackgroundError(ctx, "dequeue_fill", "pgque batch finish failed after partial claim", finishErr)
				return claimed, nil
			}
			return nil, finishErr
		}
		if err != nil {
			if len(claimed) > 0 {
				q.logBackgroundError(ctx, "dequeue_fill", "pgque claim failed after partial claim", err)
				return claimed, nil
			}
			return nil, err
		}
		if len(runs) > 0 {
			for i := range runs {
				q.runWriter.recordClaimMetrics(ctx, &runs[i])
			}
			claimed = append(claimed, runs...)
			if len(unclaimed) > 0 {
				return claimed, nil
			}
			emptyAttempts = 0
			continue
		}
		emptyAttempts++
	}
	return claimed, nil
}

func (q *PgQueQueue) nackReservedMessage(ctx context.Context, msg pgQueMessage, reason string) {
	if err := q.pgque(q.db).nack(ctx, msg, q.cfg.NackDelay, reason); err != nil {
		q.logBackgroundError(ctx, "nack", "pgque nack failed", fmt.Errorf("%s: %w", reason, err))
	}
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
	for i, msg := range batch.Messages {
		var event pgQueReadyEvent
		if err := json.Unmarshal([]byte(msg.Payload), &event); err != nil || event.RunID == "" {
			invalid = append(invalid, msg)
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
		batch.InFlight++
	}
	removeReservedMessages(batch, invalid, candidates)
	return pgQueBatchReservation{Batch: batch, Candidates: candidates, Invalid: invalid}, nil
}

func removeReservedMessages(batch *pgQueActiveBatch, invalid []pgQueMessage, candidates []pgQueCandidate) {
	if batch == nil || len(batch.Messages) == 0 {
		return
	}
	removeCount := len(invalid) + len(candidates)
	if removeCount == 0 {
		return
	}
	if removeCount == len(batch.Messages) {
		clear(batch.Messages)
		batch.Messages = batch.Messages[:0]
		return
	}
	if removeCount == 1 {
		var removeID int64
		if len(invalid) == 1 {
			removeID = invalid[0].ID
		} else {
			removeID = candidates[0].Message.ID
		}
		removeReservedMessage(batch, removeID)
		return
	}
	if removeCount <= pgQueSmallRemoveSetLimit {
		var removeIDs [pgQueSmallRemoveSetLimit]int64
		writeID := 0
		for _, msg := range invalid {
			removeIDs[writeID] = msg.ID
			writeID++
		}
		for _, candidate := range candidates {
			removeIDs[writeID] = candidate.Message.ID
			writeID++
		}

		write := 0
		for _, msg := range batch.Messages {
			if pgQueMessageIDInSet(removeIDs[:writeID], msg.ID) {
				continue
			}
			batch.Messages[write] = msg
			write++
		}
		clear(batch.Messages[write:])
		batch.Messages = batch.Messages[:write]
		return
	}

	removeIDs := make(map[int64]struct{}, removeCount)
	for _, msg := range invalid {
		removeIDs[msg.ID] = struct{}{}
	}
	for _, candidate := range candidates {
		removeIDs[candidate.Message.ID] = struct{}{}
	}

	write := 0
	for _, msg := range batch.Messages {
		if _, ok := removeIDs[msg.ID]; ok {
			continue
		}
		batch.Messages[write] = msg
		write++
	}
	clear(batch.Messages[write:])
	batch.Messages = batch.Messages[:write]
}

func pgQueMessageIDInSet(ids []int64, id int64) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func removeReservedMessage(batch *pgQueActiveBatch, removeID int64) {
	for i, msg := range batch.Messages {
		if msg.ID != removeID {
			continue
		}
		copy(batch.Messages[i:], batch.Messages[i+1:])
		last := len(batch.Messages) - 1
		batch.Messages[last] = pgQueMessage{}
		batch.Messages = batch.Messages[:last]
		return
	}
}

func (q *PgQueQueue) refreshCandidateClaimState(ctx context.Context, candidates []pgQueCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	var runIDBuffer pgQueCandidateRunIDBuffer
	runIDs := runIDBuffer.collect(candidates)

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
		runIDs,
	)
	if err != nil {
		return fmt.Errorf("pgque candidate priorities: %w", err)
	}
	defer rows.Close()

	if len(candidates) <= pgQueSmallCandidateSetLimit {
		for rows.Next() {
			var state pgQueCandidateClaimState
			if err := rows.Scan(
				&state.runID,
				&state.priority,
				&state.hasConcurrencyLimit,
			); err != nil {
				return fmt.Errorf("pgque candidate claim state scan: %w", err)
			}
			applyPgQueCandidateClaimState(candidates, state)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("pgque candidate claim state rows: %w", err)
		}
		return nil
	}

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

type pgQueCandidateRunIDBuffer struct {
	small  [pgQueSmallCandidateSetLimit]string
	values []string
}

func (b *pgQueCandidateRunIDBuffer) collect(candidates []pgQueCandidate) []string {
	if len(candidates) <= len(b.small) {
		b.values = b.small[:len(candidates)]
	} else {
		b.values = make([]string, len(candidates))
	}
	for i, candidate := range candidates {
		b.values[i] = candidate.Event.RunID
	}
	return b.values
}

func applyPgQueCandidateClaimState(candidates []pgQueCandidate, state pgQueCandidateClaimState) {
	for i := range candidates {
		if candidates[i].Event.RunID != state.runID {
			continue
		}
		candidates[i].Event.Priority = state.priority
		candidates[i].HasConcurrencyLimit = state.hasConcurrencyLimit
	}
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
	client := q.pgque(q.db)
	for range pgQueMaxCatchUpBatches {
		messages, err := client.receive(ctx, queueName, pgQueReceiveAll)
		if err != nil {
			return nil, err
		}
		if len(messages) > 0 {
			if lag, lagErr := client.consumerLag(ctx, queueName); lagErr == nil {
				recordPgQueConsumerLag(ctx, lag)
			} else {
				q.logBackgroundError(ctx, "consumer_lag", "pgque consumer lag probe failed", lagErr)
			}
			batch := &pgQueActiveBatch{BatchID: messages[0].BatchID, Messages: messages}
			state.activeBatch = batch
			return batch, nil
		}

		lag, err := client.consumerLag(ctx, queueName)
		if err != nil {
			return nil, err
		}
		recordPgQueConsumerLag(ctx, lag)
		if lag == 0 {
			return nil, errPgQueNoMessages
		}
	}
	return nil, errPgQueNoMessages
}
