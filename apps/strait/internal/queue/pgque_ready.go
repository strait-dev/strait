package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"strait/internal/domain"
	"strait/internal/store"
)

const pgQueSmallReadyWorkerJobSetLimit = 8

func (q *PgQueQueue) sendReadyEvent(ctx context.Context, db store.DBTX, run *domain.JobRun) error {
	routeKey, err := q.routeKeyForRun(ctx, db, run)
	if err != nil {
		return err
	}
	queueName := pgQueQueueName(routeKey)
	if !q.routeConfigured(routeKey) {
		if err := q.ensureRoute(ctx, db, routeKey, queueName); err != nil {
			return err
		}
	}
	generation, err := q.readyGeneration(ctx, db, run.ID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(pgQueReadyEvent{
		RunID:      run.ID,
		RouteKey:   routeKey,
		Generation: generation,
		Priority:   run.Priority,
	})
	if err != nil {
		return fmt.Errorf("pgque ready event: marshal: %w", err)
	}
	if err := q.pgque(db).sendText(ctx, queueName, pgQueReadyEventType, string(payload)); err != nil {
		return fmt.Errorf("pgque send ready event: %w", err)
	}
	if err := q.recordReadyEmitBatch(ctx, db, []string{run.ID}, []int64{generation}); err != nil {
		return err
	}
	return nil
}

func (q *PgQueQueue) sendReadyEvents(ctx context.Context, db store.DBTX, runs []*domain.JobRun) error {
	readyRuns, runIDs, err := q.readyRunsForEvents(ctx, db, runs)
	if err != nil {
		return err
	}
	if len(readyRuns) == 0 {
		return nil
	}
	generations, err := q.readyGenerations(ctx, db, runIDs)
	if err != nil {
		return err
	}

	routeKey := readyRuns[0].routeKey
	payloads := make([]string, 0, len(readyRuns))
	var byRoute map[string][]string
	readyGenerations := make([]int64, 0, len(readyRuns))
	for _, readyRun := range readyRuns {
		generation, ok := generations[readyRun.run.ID]
		if !ok {
			return fmt.Errorf("pgque ready generation: missing run %s", readyRun.run.ID)
		}
		readyGenerations = append(readyGenerations, generation)
		payload, err := json.Marshal(pgQueReadyEvent{
			RunID:      readyRun.run.ID,
			RouteKey:   readyRun.routeKey,
			Generation: generation,
			Priority:   readyRun.run.Priority,
		})
		if err != nil {
			return fmt.Errorf("pgque ready event: marshal: %w", err)
		}
		payloadText := string(payload)
		if byRoute != nil {
			byRoute[readyRun.routeKey] = append(byRoute[readyRun.routeKey], payloadText)
			continue
		}
		if readyRun.routeKey == routeKey {
			payloads = append(payloads, payloadText)
			continue
		}
		byRoute = make(map[string][]string, len(readyRuns))
		byRoute[routeKey] = payloads
		byRoute[readyRun.routeKey] = append(byRoute[readyRun.routeKey], payloadText)
	}
	if byRoute == nil {
		if err := q.sendReadyPayloadBatch(ctx, db, routeKey, payloads); err != nil {
			return err
		}
	} else {
		for routeKey, payloads := range byRoute {
			if err := q.sendReadyPayloadBatch(ctx, db, routeKey, payloads); err != nil {
				return err
			}
		}
	}
	if err := q.recordReadyEmitBatch(ctx, db, runIDs, readyGenerations); err != nil {
		return err
	}
	return nil
}

func (q *PgQueQueue) sendReadyPayloadBatch(ctx context.Context, db store.DBTX, routeKey string, payloads []string) error {
	queueName := pgQueQueueName(routeKey)
	if !q.routeConfigured(routeKey) {
		if err := q.ensureRoute(ctx, db, routeKey, queueName); err != nil {
			return err
		}
	}
	if err := q.pgque(db).sendTextBatch(ctx, queueName, pgQueReadyEventType, payloads); err != nil {
		return fmt.Errorf("pgque send ready event batch: %w", err)
	}
	return nil
}

func (q *PgQueQueue) readyRunsForEvents(ctx context.Context, db store.DBTX, runs []*domain.JobRun) ([]pgQueReadyRun, []string, error) {
	var readyRuns []pgQueReadyRun
	var runIDs []string
	var workerJobIDs []string
	var seenWorkerJobs map[string]struct{}
	for _, run := range runs {
		if run == nil || run.Status != domain.StatusQueued {
			continue
		}
		if readyRuns == nil {
			readyRuns = make([]pgQueReadyRun, 0, len(runs))
			runIDs = make([]string, 0, len(runs))
		}
		readyRuns = append(readyRuns, pgQueReadyRun{run: run})
		runIDs = append(runIDs, run.ID)
		if run.ExecutionMode != domain.ExecutionModeWorker {
			continue
		}
		if run.JobID == "" {
			return nil, nil, fmt.Errorf("pgque worker route lookup: missing job id for run %s", run.ID)
		}
		workerJobIDs, seenWorkerJobs = appendUniqueReadyWorkerJobID(workerJobIDs, seenWorkerJobs, run.JobID)
	}
	if len(workerJobIDs) == 0 {
		for i := range readyRuns {
			readyRuns[i].routeKey = pgQueHTTPRouteKey
		}
		return readyRuns, runIDs, nil
	}
	workerRoutes, err := q.workerJobRoutes(ctx, db, workerJobIDs)
	if err != nil {
		return nil, nil, err
	}
	for i := range readyRuns {
		run := readyRuns[i].run
		if run.ExecutionMode != domain.ExecutionModeWorker {
			readyRuns[i].routeKey = pgQueHTTPRouteKey
			continue
		}
		route, ok := workerRoutes[run.JobID]
		if !ok {
			return nil, nil, fmt.Errorf("pgque worker route lookup: missing job %s", run.JobID)
		}
		queueName := runQueueName(run.QueueName)
		if run.QueueName == "" {
			queueName = route.queueName
		}
		readyRuns[i].routeKey = pgQueWorkerRouteKey(run.ProjectID, queueName, route.environmentID)
	}
	return readyRuns, runIDs, nil
}

func appendUniqueReadyWorkerJobID(workerJobIDs []string, seen map[string]struct{}, jobID string) ([]string, map[string]struct{}) {
	if seen != nil {
		if _, ok := seen[jobID]; ok {
			return workerJobIDs, seen
		}
		seen[jobID] = struct{}{}
		return append(workerJobIDs, jobID), seen
	}

	for _, workerJobID := range workerJobIDs {
		if workerJobID == jobID {
			return workerJobIDs, nil
		}
	}
	workerJobIDs = append(workerJobIDs, jobID)
	if len(workerJobIDs) > pgQueSmallReadyWorkerJobSetLimit {
		seen = make(map[string]struct{}, len(workerJobIDs))
		for _, workerJobID := range workerJobIDs {
			seen[workerJobID] = struct{}{}
		}
	}
	return workerJobIDs, seen
}

type pgQueReadyRun struct {
	run      *domain.JobRun
	routeKey string
}

func (q *PgQueQueue) recordReadyEmitBatch(ctx context.Context, db store.DBTX, runIDs []string, readyGenerations []int64) error {
	if len(runIDs) == 0 {
		return nil
	}
	if len(runIDs) != len(readyGenerations) {
		return fmt.Errorf("pgque record ready emits: mismatched id/generation counts")
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO strait_pgque_ready_events (run_id, ready_generation)
		SELECT run_id, ready_generation
		FROM unnest($1::text[], $2::bigint[]) AS input(run_id, ready_generation)
		ON CONFLICT (run_id, ready_generation) DO NOTHING`, runIDs, readyGenerations); err != nil {
		return fmt.Errorf("pgque record ready emits: %w", err)
	}
	return nil
}

func (q *PgQueQueue) tickReadyRoute(ctx context.Context, run *domain.JobRun) error {
	routeKey, err := q.routeKeyForRun(ctx, q.db, run)
	if err != nil {
		return err
	}
	queueName := pgQueQueueName(routeKey)
	if err := q.pgque(q.db).ticker(ctx, queueName); err != nil {
		return fmt.Errorf("pgque tick ready route %s: %w", routeKey, err)
	}
	return nil
}
