package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"strait/internal/domain"
	"strait/internal/store"
)

const (
	pgQueHTTPRouteKey = "http"
	pgQueQueuePrefix  = "stq_"
)

func pgQueQueueName(routeKey string) string {
	sum := sha256.Sum256([]byte(routeKey))
	return pgQueQueuePrefix + hex.EncodeToString(sum[:])[:32]
}

func pgQueRouteKeyForRun(run *domain.JobRun) string {
	if run != nil && run.ExecutionMode == domain.ExecutionModeWorker {
		return pgQueWorkerRouteKey(run.ProjectID, runQueueName(run.QueueName), "")
	}
	return pgQueHTTPRouteKey
}

func pgQueWorkerRouteKey(projectID, queueName, environmentID string) string {
	return strings.Join([]string{
		"worker",
		projectID,
		runQueueName(queueName),
		environmentID,
	}, ":")
}

func normalizePgQueWorkerQueueRefs(refs []domain.WorkerQueueRef) []domain.WorkerQueueRef {
	if len(refs) == 0 {
		return nil
	}
	normalized := make([]domain.WorkerQueueRef, 0, len(refs))
	seen := make(map[domain.WorkerQueueRef]struct{}, len(refs))
	for _, ref := range refs {
		if ref.ProjectID == "" || ref.QueueName == "" {
			continue
		}
		ref.QueueName = runQueueName(ref.QueueName)
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		normalized = append(normalized, ref)
	}
	return normalized
}

func workerQueueRefArgs(refs []domain.WorkerQueueRef) ([]string, []string, []string) {
	if len(refs) == 0 {
		return nil, nil, nil
	}
	projectIDs := make([]string, 0, len(refs))
	queueNames := make([]string, 0, len(refs))
	environmentIDs := make([]string, 0, len(refs))
	seen := make(map[domain.WorkerQueueRef]struct{}, len(refs))
	for _, ref := range refs {
		if ref.ProjectID == "" || ref.QueueName == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		projectIDs = append(projectIDs, ref.ProjectID)
		queueNames = append(queueNames, ref.QueueName)
		environmentIDs = append(environmentIDs, ref.EnvironmentID)
	}
	return projectIDs, queueNames, environmentIDs
}

func (q *PgQueQueue) routeKeyForRun(ctx context.Context, db store.DBTX, run *domain.JobRun) (string, error) {
	if run == nil || run.ExecutionMode != domain.ExecutionModeWorker {
		return pgQueHTTPRouteKey, nil
	}
	var queueName, environmentID string
	if err := db.QueryRow(ctx, `
		SELECT COALESCE(NULLIF($2, ''), NULLIF(j.queue_name, ''), 'default'),
		       COALESCE(j.environment_id, '')
		FROM jobs j
		WHERE j.id = $1`, run.JobID, run.QueueName).Scan(&queueName, &environmentID); err != nil {
		return "", fmt.Errorf("pgque worker route lookup: %w", err)
	}
	return pgQueWorkerRouteKey(run.ProjectID, queueName, environmentID), nil
}

func (q *PgQueQueue) readyGeneration(ctx context.Context, db store.DBTX, runID string) (int64, error) {
	var generation int64
	if err := db.QueryRow(ctx, `SELECT ready_generation FROM job_run_state WHERE run_id = $1`, runID).Scan(&generation); err != nil {
		return 0, fmt.Errorf("pgque ready generation: %w", err)
	}
	return generation, nil
}

func (q *PgQueQueue) readyGenerations(ctx context.Context, db store.DBTX, runIDs []string) (map[string]int64, error) {
	generations := make(map[string]int64, len(runIDs))
	if len(runIDs) == 0 {
		return generations, nil
	}
	rows, err := db.Query(ctx, `
		SELECT run_id, ready_generation
		FROM job_run_state
		WHERE run_id = ANY($1::text[])`, runIDs)
	if err != nil {
		return nil, fmt.Errorf("pgque ready generations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var runID string
		var generation int64
		if err := rows.Scan(&runID, &generation); err != nil {
			return nil, fmt.Errorf("pgque ready generations scan: %w", err)
		}
		generations[runID] = generation
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque ready generations rows: %w", err)
	}
	return generations, nil
}

func (q *PgQueQueue) workerRouteKeys(ctx context.Context, refs []domain.WorkerQueueRef) ([]string, error) {
	routes := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		queueName := runQueueName(ref.QueueName)
		if ref.EnvironmentID != "" {
			key := pgQueWorkerRouteKey(ref.ProjectID, queueName, ref.EnvironmentID)
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				routes = append(routes, key)
			}
			continue
		}
		prefix := pgQueWorkerRouteKey(ref.ProjectID, queueName, "")
		var err error
		routes, err = q.appendKnownWorkerRoutes(ctx, prefix, seen, routes)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[prefix]; !ok {
			seen[prefix] = struct{}{}
			routes = append(routes, prefix)
		}
	}
	return routes, nil
}

func (q *PgQueQueue) appendKnownWorkerRoutes(
	ctx context.Context,
	prefix string,
	seen map[string]struct{},
	routes []string,
) ([]string, error) {
	rows, err := q.db.Query(ctx, `
		SELECT route_key
		FROM strait_pgque_routes
		WHERE route_key = $1 OR route_key LIKE $2
		ORDER BY route_key`, prefix, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("pgque worker route lookup: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("pgque worker route scan: %w", err)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		routes = append(routes, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque worker route rows: %w", err)
	}
	return routes, nil
}
