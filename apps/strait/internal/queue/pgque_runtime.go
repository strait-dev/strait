package queue

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func (q *PgQueQueue) nextWorkerRouteStart(routeCount int) int {
	if routeCount <= 1 {
		return 0
	}
	return int((q.workerRouteCursor.Add(1) - 1) % uint64(routeCount))
}

func (q *PgQueQueue) ForceTick(ctx context.Context, routeKey string) error {
	queueName := pgQueQueueName(routeKey)
	return q.forceTickQueue(ctx, queueName)
}

func (q *PgQueQueue) forceTickQueue(ctx context.Context, queueName string) error {
	client := q.pgque(q.db)
	if err := client.forceNextTick(ctx, queueName); err != nil {
		return err
	}
	return client.ticker(ctx, queueName)
}

func (q *PgQueQueue) routeState(routeKey string) *pgQueRouteState {
	q.routeMu.Lock()
	defer q.routeMu.Unlock()
	state := q.routeStates[routeKey]
	if state == nil {
		state = &pgQueRouteState{}
		q.routeStates[routeKey] = state
	}
	return state
}

func (q *PgQueQueue) routeConfigured(routeKey string) bool {
	state := q.routeState(routeKey)
	return state.configured.Load()
}

func (q *PgQueQueue) ensureRunRouteCached(ctx context.Context, run *domain.JobRun) error {
	routeKey, err := q.routeKeyForRun(ctx, q.db, run)
	if err != nil {
		return err
	}
	queueName := pgQueQueueName(routeKey)
	state := q.routeState(routeKey)
	return q.ensureRouteCached(ctx, state, routeKey, queueName)
}

func (q *PgQueQueue) ensureRunRoutesCached(ctx context.Context, runs []*domain.JobRun) error {
	routeSet := pgQueRouteEnsureSet{}

	var workerRuns []*domain.JobRun
	var workerJobIDs []string
	var seenWorkerJobs map[string]struct{}
	for _, run := range runs {
		if run == nil || run.Status != domain.StatusQueued {
			continue
		}
		if run.ExecutionMode != domain.ExecutionModeWorker {
			if err := q.ensureCachedRouteKeyOnce(ctx, &routeSet, pgQueHTTPRouteKey); err != nil {
				return err
			}
			continue
		}
		if run.JobID == "" {
			return fmt.Errorf("pgque worker route lookup: missing job id for run %s", run.ID)
		}
		if workerRuns == nil {
			workerRuns = make([]*domain.JobRun, 0, len(runs))
		}
		workerRuns = append(workerRuns, run)
		workerJobIDs, seenWorkerJobs = appendUniqueReadyWorkerJobID(workerJobIDs, seenWorkerJobs, run.JobID)
	}
	if len(workerRuns) == 0 {
		return nil
	}
	workerRoutes, err := q.workerJobRoutes(ctx, q.db, workerJobIDs)
	if err != nil {
		return err
	}
	for _, run := range workerRuns {
		route, ok := workerRoutes[run.JobID]
		if !ok {
			return fmt.Errorf("pgque worker route lookup: missing job %s", run.JobID)
		}
		queueName := runQueueName(run.QueueName)
		if run.QueueName == "" {
			queueName = route.queueName
		}
		routeKey := pgQueWorkerRouteKey(run.ProjectID, queueName, route.environmentID)
		if err := q.ensureCachedRouteKeyOnce(ctx, &routeSet, routeKey); err != nil {
			return err
		}
	}
	return nil
}

type pgQueRouteEnsureSet struct {
	smallRoutes [pgQueSmallRouteSetLimit]string
	routeCount  int
	seen        map[string]struct{}
}

func (q *PgQueQueue) ensureCachedRouteKeyOnce(
	ctx context.Context,
	routeSet *pgQueRouteEnsureSet,
	routeKey string,
) error {
	if routeSet.seen != nil {
		if _, ok := routeSet.seen[routeKey]; ok {
			return nil
		}
		routeSet.seen[routeKey] = struct{}{}
	} else if containsRoute(routeSet.smallRoutes[:routeSet.routeCount], routeKey) {
		return nil
	} else if routeSet.routeCount < len(routeSet.smallRoutes) {
		routeSet.smallRoutes[routeSet.routeCount] = routeKey
		routeSet.routeCount++
	} else {
		routeSet.seen = make(map[string]struct{}, routeSet.routeCount+1)
		for _, existing := range routeSet.smallRoutes {
			routeSet.seen[existing] = struct{}{}
		}
		routeSet.seen[routeKey] = struct{}{}
	}
	queueName := pgQueQueueName(routeKey)
	state := q.routeState(routeKey)
	if err := q.ensureRouteCached(ctx, state, routeKey, queueName); err != nil {
		return err
	}
	return nil
}

func (q *PgQueQueue) ensureRouteCached(ctx context.Context, state *pgQueRouteState, routeKey, queueName string) error {
	if state.configured.Load() {
		return nil
	}
	state.configMu.Lock()
	defer state.configMu.Unlock()
	if state.configured.Load() {
		return nil
	}
	if err := q.ensureRoute(ctx, q.db, routeKey, queueName); err != nil {
		return err
	}
	state.configured.Store(true)
	return nil
}

func (q *PgQueQueue) ensureRoute(ctx context.Context, db store.DBTX, routeKey, queueName string) error {
	if _, err := db.Exec(ctx, `
		INSERT INTO strait_pgque_routes (route_key, queue_name)
		VALUES ($1, $2)
		ON CONFLICT (route_key) DO NOTHING`, routeKey, queueName); err != nil {
		return fmt.Errorf("pgque route upsert: %w", err)
	}
	q.invalidateWorkerRouteCache(routeKey)
	client := q.pgque(db)
	if err := client.createQueue(ctx, queueName); err != nil {
		return err
	}
	if err := client.setQueueConfig(ctx, queueName, "ticker_max_count", strconv.Itoa(q.cfg.ReceiveWindow)); err != nil {
		return err
	}
	rotationPeriod := pgQueIntervalSetting(q.cfg.RotationPeriod)
	if err := client.setQueueConfig(ctx, queueName, "rotation_period", rotationPeriod); err != nil {
		return err
	}
	return client.registerConsumer(ctx, queueName)
}

func (q *PgQueQueue) maybeForceTick(ctx context.Context, state *pgQueRouteState, queueName string) {
	if !state.lastForceTick.IsZero() && time.Since(state.lastForceTick) < q.cfg.TickInterval {
		return
	}
	if err := q.forceTickQueue(ctx, queueName); err == nil {
		state.lastForceTick = time.Now()
	}
}

func (q *PgQueQueue) RunTicker(ctx context.Context) {
	ticker := time.NewTicker(q.cfg.TickInterval)
	defer ticker.Stop()
	maintenance := time.NewTicker(q.cfg.MaintenanceInterval)
	defer maintenance.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := q.pgque(q.db).tickerAll(ctx); err != nil {
				q.logBackgroundError(ctx, "ticker", "pgque ticker failed", err)
			}
		case <-maintenance.C:
			if err := q.Maintain(ctx); err != nil {
				q.logBackgroundError(ctx, "maintenance", "pgque maintenance failed", err)
			}
		}
	}
}

func (q *PgQueQueue) logBackgroundError(ctx context.Context, operation, message string, err error) {
	if err == nil || ctx.Err() != nil {
		return
	}
	if qm, metricErr := Metrics(); metricErr == nil && qm != nil {
		attrs := metric.WithAttributes(
			attribute.String("operation", pgQueBackgroundOperationLabel(operation)),
		)
		qm.PgQueBackgroundErrors.Add(ctx, 1, attrs)
	}
	logger := q.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Warn(message, "error", err)
}

func recordPgQueConsumerLag(ctx context.Context, lag int64) {
	if ctx.Err() != nil {
		return
	}
	if qm, metricErr := Metrics(); metricErr == nil && qm != nil {
		qm.PgQueConsumerLag.Record(ctx, lag)
	}
}

func pgQueBackgroundOperationLabel(operation string) string {
	switch operation {
	case "ticker", "maintenance", "nack":
		return operation
	default:
		return "other"
	}
}

func (q *PgQueQueue) Maintain(ctx context.Context) error {
	rotationQueues, err := q.rotationQueuesDueForMaintenance(ctx)
	if err != nil {
		return err
	}
	client := q.pgque(q.db)
	for _, queueName := range rotationQueues {
		if err := client.rotateTablesStep1(ctx, queueName); err != nil {
			return err
		}
	}
	return client.rotateTablesStep2(ctx)
}

func (q *PgQueQueue) rotationQueuesDueForMaintenance(ctx context.Context) ([]string, error) {
	rows, err := q.db.Query(ctx, `
		SELECT func_arg
		FROM pgque.maint_operations()
		WHERE func_name = 'pgque.maint_rotate_tables_step1'
		  AND func_arg IS NOT NULL
		ORDER BY func_arg`)
	if err != nil {
		return nil, fmt.Errorf("pgque maintain operations: %w", err)
	}
	defer rows.Close()

	queueNames := []string{}
	for rows.Next() {
		var queueName string
		if err := rows.Scan(&queueName); err != nil {
			return nil, fmt.Errorf("pgque maintain operation scan: %w", err)
		}
		queueNames = append(queueNames, queueName)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("pgque maintain operations rows: %w", err)
	}
	return queueNames, nil
}

func pgQueIntervalSetting(d time.Duration) string {
	if d <= 0 {
		d = pgQueDefaultRotationPeriod
	}
	return strconv.FormatInt(d.Microseconds(), 10) + " microseconds"
}
