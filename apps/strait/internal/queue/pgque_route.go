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
	pgQueSmallWorkerRefLimit = 8
	pgQueWorkerRouteCacheTTL = time.Second
)

func pgQueQueueName(routeKey string) string {
	sum := sha256.Sum256([]byte(routeKey))
	var queueName [len(pgQueQueuePrefix) + 32]byte
	copy(queueName[:], pgQueQueuePrefix)
	hex.Encode(queueName[len(pgQueQueuePrefix):], sum[:16])
	return string(queueName[:])
}

func pgQueRouteKeyForRun(run *domain.JobRun) string {
	if run != nil && run.ExecutionMode == domain.ExecutionModeWorker {
		return pgQueWorkerRouteKey(run.ProjectID, runQueueName(run.QueueName), "")
	}
	return pgQueHTTPRouteKey
}

func pgQueWorkerRouteKey(projectID, queueName, environmentID string) string {
	queueName = runQueueName(queueName)
	var b strings.Builder
	b.Grow(len("worker::") + len(projectID) + len(queueName) + len(environmentID))
	b.WriteString("worker:")
	b.WriteString(projectID)
	b.WriteByte(':')
	b.WriteString(queueName)
	b.WriteByte(':')
	b.WriteString(environmentID)
	return b.String()
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
	if !strings.HasPrefix(routeKey, "worker:") {
		return domain.WorkerQueueRef{}, false
	}
	rest := routeKey[len("worker:"):]
	projectEnd := strings.IndexByte(rest, ':')
	if projectEnd <= 0 {
		return domain.WorkerQueueRef{}, false
	}
	projectID := rest[:projectEnd]
	rest = rest[projectEnd+1:]
	queueEnd := strings.IndexByte(rest, ':')
	if queueEnd <= 0 {
		return domain.WorkerQueueRef{}, false
	}
	queueName := rest[:queueEnd]
	environmentID := rest[queueEnd+1:]
	if strings.Contains(environmentID, ":") {
		return domain.WorkerQueueRef{}, false
	}
	return domain.WorkerQueueRef{
		ProjectID:     projectID,
		QueueName:     runQueueName(queueName),
		EnvironmentID: environmentID,
	}, true
}

func normalizePgQueWorkerQueueRefs(refs []domain.WorkerQueueRef) []domain.WorkerQueueRef {
	if len(refs) == 0 {
		return nil
	}
	if len(refs) == 1 {
		ref := refs[0]
		if ref.ProjectID == "" || ref.QueueName == "" {
			return nil
		}
		return refs
	}
	normalized := make([]domain.WorkerQueueRef, 0, len(refs))
	var seen map[domain.WorkerQueueRef]struct{}
	for _, ref := range refs {
		if ref.ProjectID == "" || ref.QueueName == "" {
			continue
		}
		ref.QueueName = runQueueName(ref.QueueName)
		normalized, seen = appendUniqueWorkerQueueRef(normalized, seen, ref)
	}
	return normalized
}

func appendUniqueWorkerQueueRef(
	refs []domain.WorkerQueueRef,
	seen map[domain.WorkerQueueRef]struct{},
	ref domain.WorkerQueueRef,
) ([]domain.WorkerQueueRef, map[domain.WorkerQueueRef]struct{}) {
	if seen != nil {
		if _, ok := seen[ref]; ok {
			return refs, seen
		}
		seen[ref] = struct{}{}
		return append(refs, ref), seen
	}

	for _, existing := range refs {
		if existing == ref {
			return refs, nil
		}
	}
	refs = append(refs, ref)
	if len(refs) > pgQueSmallWorkerRefLimit {
		seen = make(map[domain.WorkerQueueRef]struct{}, len(refs))
		for _, existing := range refs {
			seen[existing] = struct{}{}
		}
	}
	return refs, seen
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
	if len(refs) == 1 {
		ref := refs[0]
		if ref.ProjectID == "" || ref.QueueName == "" {
			return pgQueWorkerRefArgs{}
		}
		args := makeWorkerRefArgs(1)
		args.ProjectIDs[0] = ref.ProjectID
		args.QueueNames[0] = ref.QueueName
		args.EnvironmentIDs[0] = ref.EnvironmentID
		return args
	}
	args := makeWorkerRefArgs(len(refs))
	var smallRefs [pgQueSmallWorkerRefLimit]domain.WorkerQueueRef
	smallRefCount := 0
	argCount := 0
	var seen map[domain.WorkerQueueRef]struct{}
	for _, ref := range refs {
		if ref.ProjectID == "" || ref.QueueName == "" {
			continue
		}
		if workerQueueRefSeen(ref, smallRefs[:smallRefCount], seen) {
			continue
		}
		if seen != nil {
			seen[ref] = struct{}{}
		} else if smallRefCount < len(smallRefs) {
			smallRefs[smallRefCount] = ref
			smallRefCount++
		} else {
			seen = make(map[domain.WorkerQueueRef]struct{}, len(refs))
			for _, existing := range smallRefs {
				seen[existing] = struct{}{}
			}
			seen[ref] = struct{}{}
		}
		args.ProjectIDs[argCount] = ref.ProjectID
		args.QueueNames[argCount] = ref.QueueName
		args.EnvironmentIDs[argCount] = ref.EnvironmentID
		argCount++
	}
	return trimWorkerRefArgs(args, argCount)
}

func workerQueueRefSeen(
	ref domain.WorkerQueueRef,
	smallRefs []domain.WorkerQueueRef,
	seen map[domain.WorkerQueueRef]struct{},
) bool {
	if seen != nil {
		_, ok := seen[ref]
		return ok
	}
	for _, existing := range smallRefs {
		if existing == ref {
			return true
		}
	}
	return false
}

func workerQueueRefArgsFromNormalized(refs []domain.WorkerQueueRef) pgQueWorkerRefArgs {
	if len(refs) == 0 {
		return pgQueWorkerRefArgs{}
	}
	args := makeWorkerRefArgs(len(refs))
	for i, ref := range refs {
		args.ProjectIDs[i] = ref.ProjectID
		args.QueueNames[i] = ref.QueueName
		args.EnvironmentIDs[i] = ref.EnvironmentID
	}
	return args
}

func makeWorkerRefArgs(size int) pgQueWorkerRefArgs {
	if size <= 0 {
		return pgQueWorkerRefArgs{}
	}
	values := make([]string, size*3)
	return pgQueWorkerRefArgs{
		ProjectIDs:     values[:size:size],
		QueueNames:     values[size : 2*size : 2*size],
		EnvironmentIDs: values[2*size : 3*size : 3*size],
	}
}

func trimWorkerRefArgs(args pgQueWorkerRefArgs, size int) pgQueWorkerRefArgs {
	if size <= 0 {
		return pgQueWorkerRefArgs{}
	}
	return pgQueWorkerRefArgs{
		ProjectIDs:     args.ProjectIDs[:size:size],
		QueueNames:     args.QueueNames[:size:size],
		EnvironmentIDs: args.EnvironmentIDs[:size:size],
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

func (q *PgQueQueue) workerJobRoute(ctx context.Context, db store.DBTX, jobID string) (pgQueWorkerJobRoute, bool, error) {
	rows, err := db.Query(ctx, `
		SELECT id,
		       COALESCE(NULLIF(queue_name, ''), 'default'),
		       COALESCE(environment_id, '')
		FROM jobs
		WHERE id = $1`, jobID)
	if err != nil {
		return pgQueWorkerJobRoute{}, false, fmt.Errorf("pgque worker route lookup: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return pgQueWorkerJobRoute{}, false, fmt.Errorf("pgque worker route rows: %w", err)
		}
		return pgQueWorkerJobRoute{}, false, nil
	}

	var gotJobID string
	var route pgQueWorkerJobRoute
	if err := rows.Scan(&gotJobID, &route.queueName, &route.environmentID); err != nil {
		return pgQueWorkerJobRoute{}, false, fmt.Errorf("pgque worker route scan: %w", err)
	}
	if err := rows.Err(); err != nil {
		return pgQueWorkerJobRoute{}, false, fmt.Errorf("pgque worker route rows: %w", err)
	}
	return route, true, nil
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
			seen = make(map[string]struct{}, routeSeenCapacity(routes, candidates))
			for _, routeKey := range routes {
				seen[routeKey] = struct{}{}
			}
		}
	}
	return routes, seen
}

func routeSeenCapacity(routes, candidates []string) int {
	return len(routes) + len(candidates)
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
		WHERE route_key = $1 OR route_key LIKE $2 ESCAPE '\'
		ORDER BY route_key`, prefix, store.EscapePostgresLikePattern(prefix)+"%")
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
