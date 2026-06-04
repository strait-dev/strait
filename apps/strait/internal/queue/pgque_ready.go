package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"strait/internal/domain"
	"strait/internal/store"
)

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
	readyRuns := make([]pgQueReadyRun, 0, len(runs))
	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		if run == nil || run.Status != domain.StatusQueued {
			continue
		}
		routeKey, err := q.routeKeyForRun(ctx, db, run)
		if err != nil {
			return err
		}
		readyRuns = append(readyRuns, pgQueReadyRun{
			run:      run,
			routeKey: routeKey,
		})
		runIDs = append(runIDs, run.ID)
	}
	generations, err := q.readyGenerations(ctx, db, runIDs)
	if err != nil {
		return err
	}

	byRoute := make(map[string][]string, len(readyRuns))
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
		byRoute[readyRun.routeKey] = append(byRoute[readyRun.routeKey], string(payload))
	}
	for routeKey, payloads := range byRoute {
		queueName := pgQueQueueName(routeKey)
		if !q.routeConfigured(routeKey) {
			if err := q.ensureRoute(ctx, db, routeKey, queueName); err != nil {
				return err
			}
		}
		if err := q.pgque(db).sendTextBatch(ctx, queueName, pgQueReadyEventType, payloads); err != nil {
			return fmt.Errorf("pgque send ready event batch: %w", err)
		}
	}
	if err := q.recordReadyEmitBatch(ctx, db, runIDs, readyGenerations); err != nil {
		return err
	}
	return nil
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
