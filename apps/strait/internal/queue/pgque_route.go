package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

const (
	pgQueHTTPRouteKey        = "http"
	pgQueQueuePrefix         = "stq_"
	pgQueSmallRouteSetLimit  = 8
	pgQueWorkerRouteCacheTTL = time.Second
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

func pgQueWorkerRoutePrefix(routeKey string) string {
	if !strings.HasPrefix(routeKey, "worker:") {
		return ""
	}
	lastColon := strings.LastIndex(routeKey, ":")
	if lastColon < len("worker:") {
		return ""
	}
	return routeKey[:lastColon+1]
}

func pgQueWorkerRouteRef(routeKey string) (domain.WorkerQueueRef, bool) {
	parts := strings.Split(routeKey, ":")
	if len(parts) != 4 || parts[0] != "worker" || parts[1] == "" || parts[2] == "" {
		return domain.WorkerQueueRef{}, false
	}
	return domain.WorkerQueueRef{
		ProjectID:     parts[1],
		QueueName:     runQueueName(parts[2]),
		EnvironmentID: parts[3],
	}, true
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

type pgQueWorkerRefArgs struct {
	ProjectIDs     []string
	QueueNames     []string
	EnvironmentIDs []string
}

func (filter pgQueClaimFilter) workerArgs() pgQueWorkerRefArgs {
	if len(filter.WorkerRefs) == 0 || len(filter.workerRefArgs.ProjectIDs) > 0 {
		return filter.workerRefArgs
	}
	return workerQueueRefArgs(filter.WorkerRefs)
}

func workerQueueRefArgs(refs []domain.WorkerQueueRef) pgQueWorkerRefArgs {
	if len(refs) == 0 {
		return pgQueWorkerRefArgs{}
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
	return pgQueWorkerRefArgs{
		ProjectIDs:     projectIDs,
		QueueNames:     queueNames,
		EnvironmentIDs: environmentIDs,
	}
}

func workerQueueRefArgsFromNormalized(refs []domain.WorkerQueueRef) pgQueWorkerRefArgs {
	if len(refs) == 0 {
		return pgQueWorkerRefArgs{}
	}
	projectIDs := make([]string, len(refs))
	queueNames := make([]string, len(refs))
	environmentIDs := make([]string, len(refs))
	for i, ref := range refs {
		projectIDs[i] = ref.ProjectID
		queueNames[i] = ref.QueueName
		environmentIDs[i] = ref.EnvironmentID
	}
	return pgQueWorkerRefArgs{
		ProjectIDs:     projectIDs,
		QueueNames:     queueNames,
		EnvironmentIDs: environmentIDs,
	}
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

type pgQueWorkerJobRoute struct {
	queueName     string
	environmentID string
}

func (q *PgQueQueue) workerJobRoutes(ctx context.Context, db store.DBTX, jobIDs []string) (map[string]pgQueWorkerJobRoute, error) {
	routes := make(map[string]pgQueWorkerJobRoute, len(jobIDs))
	if len(jobIDs) == 0 {
		return routes, nil
	}
	rows, err := db.Query(ctx, `
		SELECT id,
		       COALESCE(NULLIF(queue_name, ''), 'default'),
		       COALESCE(environment_id, '')
		FROM jobs
		WHERE id = ANY($1::text[])`, jobIDs)
	if err != nil {
		return nil, fmt.Errorf("pgque worker route lookup: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var jobID string
		var route pgQueWorkerJobRoute
		if err := rows.Scan(&jobID, &route.queueName, &route.environmentID); err != nil {
			return nil, fmt.Errorf("pgque worker route scan: %w", err)
		}
		routes[jobID] = route
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque worker route rows: %w", err)
	}
	return routes, nil
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
	if len(refs) == 1 {
		return q.workerRouteKeysForSingleRef(ctx, refs[0])
	}
	var smallRouteGroups [pgQueSmallRouteSetLimit][]string
	routeGroups := smallRouteGroups[:0]
	if len(refs) > len(smallRouteGroups) {
		routeGroups = make([][]string, 0, len(refs))
	}
	totalRoutes := 0
	for _, ref := range refs {
		knownRoutes, err := q.workerRouteKeysForSingleRef(ctx, ref)
		if err != nil {
			return nil, err
		}
		if len(knownRoutes) == 0 {
			continue
		}
		totalRoutes += len(knownRoutes)
		routeGroups = append(routeGroups, knownRoutes)
	}
	routes := make([]string, 0, totalRoutes)
	var seen map[string]struct{}
	for _, knownRoutes := range routeGroups {
		routes, seen = appendUniqueRouteKeys(routes, seen, knownRoutes)
	}
	return routes, nil
}

func appendUniqueRouteKeys(routes []string, seen map[string]struct{}, candidates []string) ([]string, map[string]struct{}) {
	for _, key := range candidates {
		if seen != nil {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			routes = append(routes, key)
			continue
		}
		if containsRoute(routes, key) {
			continue
		}
		routes = append(routes, key)
		if len(routes) > pgQueSmallRouteSetLimit {
			seen = make(map[string]struct{}, len(routes)+len(candidates))
			for _, routeKey := range routes {
				seen[routeKey] = struct{}{}
			}
		}
	}
	return routes, seen
}

func (q *PgQueQueue) workerRouteKeysForSingleRef(ctx context.Context, ref domain.WorkerQueueRef) ([]string, error) {
	if ref.ProjectID == "" || ref.QueueName == "" {
		return nil, nil
	}
	ref.QueueName = runQueueName(ref.QueueName)
	if cached := q.cachedWorkerRoutesForRef(ref); cached != nil {
		return cached, nil
	}
	var routes []string
	var err error
	if ref.EnvironmentID != "" {
		routes = []string{pgQueWorkerRouteKey(ref.ProjectID, ref.QueueName, ref.EnvironmentID)}
	} else {
		routes, err = q.workerRoutesForPrefix(ctx, pgQueWorkerRouteKey(ref.ProjectID, ref.QueueName, ""))
		if err != nil {
			return nil, err
		}
	}
	q.cacheWorkerRoutesForRef(ref, routes)
	return routes, nil
}

func (q *PgQueQueue) workerRoutesForPrefix(ctx context.Context, prefix string) ([]string, error) {
	if cached := q.cachedWorkerRoutes(prefix); cached != nil {
		return cached, nil
	}

	routes, err := q.loadWorkerRoutesForPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	if !containsRoute(routes, prefix) {
		routes = append(routes, prefix)
	}
	q.cacheWorkerRoutes(prefix, routes)
	return routes, nil
}

func (q *PgQueQueue) cachedWorkerRoutes(prefix string) []string {
	now := time.Now()
	q.routeMu.Lock()
	defer q.routeMu.Unlock()
	entry, ok := q.routeCache[prefix]
	if !ok {
		return nil
	}
	if !now.Before(entry.expiresAt) {
		delete(q.routeCache, prefix)
		return nil
	}
	// Route cache entries are immutable after storage. Returning the stored
	// slice avoids allocating on every worker dequeue poll.
	return entry.routes
}

func (q *PgQueQueue) cachedWorkerRoutesForRef(ref domain.WorkerQueueRef) []string {
	now := time.Now()
	q.routeMu.Lock()
	defer q.routeMu.Unlock()
	entry, ok := q.routeRefCache[ref]
	if !ok {
		return nil
	}
	if !now.Before(entry.expiresAt) {
		delete(q.routeRefCache, ref)
		return nil
	}
	return entry.routes
}

func (q *PgQueQueue) cacheWorkerRoutes(prefix string, routes []string) {
	q.routeMu.Lock()
	defer q.routeMu.Unlock()
	q.routeCache[prefix] = pgQueRouteCacheEntry{
		routes:    append([]string{}, routes...),
		expiresAt: time.Now().Add(pgQueWorkerRouteCacheTTL),
	}
}

func (q *PgQueQueue) cacheWorkerRoutesForRef(ref domain.WorkerQueueRef, routes []string) {
	q.routeMu.Lock()
	defer q.routeMu.Unlock()
	q.routeRefCache[ref] = pgQueRouteCacheEntry{
		routes:    append([]string{}, routes...),
		expiresAt: time.Now().Add(pgQueWorkerRouteCacheTTL),
	}
}

func (q *PgQueQueue) invalidateWorkerRouteCache(routeKey string) {
	prefix := pgQueWorkerRoutePrefix(routeKey)
	if prefix == "" {
		return
	}
	q.routeMu.Lock()
	defer q.routeMu.Unlock()
	delete(q.routeCache, prefix)
	if ref, ok := pgQueWorkerRouteRef(routeKey); ok {
		delete(q.routeRefCache, ref)
		ref.EnvironmentID = ""
		delete(q.routeRefCache, ref)
	}
}

func (q *PgQueQueue) loadWorkerRoutesForPrefix(ctx context.Context, prefix string) ([]string, error) {
	rows, err := q.db.Query(ctx, `
		SELECT route_key
		FROM strait_pgque_routes
		WHERE route_key = $1 OR route_key LIKE $2
		ORDER BY route_key`, prefix, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("pgque worker route lookup: %w", err)
	}
	defer rows.Close()

	routes := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("pgque worker route scan: %w", err)
		}
		routes = append(routes, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque worker route rows: %w", err)
	}
	return routes, nil
}

func containsRoute(routes []string, route string) bool {
	for _, candidate := range routes {
		if candidate == route {
			return true
		}
	}
	return false
}
