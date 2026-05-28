package worker

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/telemetry"

	"github.com/getsentry/sentry-go"
)

// twoPhaseDequeuer claims IDs first, then fetches full rows by PK.
type twoPhaseDequeuer interface {
	DequeueNTwoPhase(ctx context.Context, n int) ([]domain.JobRun, error)
}

// fullyDenormalizedDequeuer is the optional interface for the legacy
// job_runs-only dequeue path.
type fullyDenormalizedDequeuer interface {
	DequeueNFullyDenormalized(ctx context.Context, n int) ([]domain.JobRun, error)
}

// claimTableDequeuer deletes from job_run_queue, then fetches from job_runs.
type claimTableDequeuer interface {
	DequeueNClaim(ctx context.Context, n int) ([]domain.JobRun, error)
}

// workerQueueDequeuer claims worker-mode runs for specific queues.
type workerQueueDequeuer interface {
	DequeueNForWorkerQueues(ctx context.Context, n int, queues []domain.WorkerQueueRef) ([]domain.JobRun, error)
}

type partitionedDequeuer interface {
	DequeueNPartitioned(ctx context.Context, n int, projectIDs []string) ([]domain.JobRun, error)
}

// QueueSnapshotter returns the environment-qualified queues that have active
// workers connected to this replica. Implemented by grpc.ConnectionRegistry via
// an adapter to avoid a circular import.
type QueueSnapshotter interface {
	SnapshotWorkerQueues() []domain.WorkerQueueRef
}

func (e *Executor) poll(ctx context.Context) {
	start := time.Now()
	if e.checkMemoryPressure() {
		return
	}
	available := e.computeAvailable()
	if available <= 0 {
		return
	}

	// Short-circuit when the DB circuit breaker is open so
	// we don't pile up goroutines on a slow Postgres.
	if e.dbCircuit != nil && e.dbCircuit.State() == queue.CircuitOpen {
		e.logger.Debug("poll skipped: db circuit open")
		return
	}

	var runs []domain.JobRun
	var err error

	dequeueErr := e.dbCircuit.Do(ctx, func(innerCtx context.Context) error {
		runs, err = e.dequeueRuns(innerCtx, available)
		return err
	})
	if e.metrics != nil {
		e.metrics.DequeueDuration.Record(ctx, time.Since(start).Seconds())
	}
	if dequeueErr != nil {
		if errors.Is(dequeueErr, queue.ErrCircuitOpen) {
			e.logger.Debug("poll: db circuit open, skipping")
		} else {
			e.logger.Error("dequeue failed", "error", dequeueErr)
		}
		return
	}
	if len(runs) == 0 {
		return
	}

	// Capture the claim time as close to DequeueN's return as possible so
	// the ClaimToStart histogram measures the real gap between a run being
	// claimed and user work starting, not the write time of StartedAt (which
	// DequeueN sets inside its UPDATE and is therefore a mix of commit
	// latency and clock skew).
	claimedAt := time.Now()

	e.logger.Info("dequeued runs", "count", len(runs))

	for i := range runs {
		run := runs[i]
		e.logger.Info(
			"dequeued run",
			"run_id", run.ID,
			"job_id", run.JobID,
			"project_id", run.ProjectID,
			"attempt", run.Attempt,
			"priority", run.Priority,
		)

		execCtx := telemetry.EnsureSentryHub(withDispatchCache(context.WithoutCancel(ctx)))
		addWorkerRunBreadcrumb(execCtx, "queue.claim", "run claimed", &run, nil, map[string]any{
			"priority": run.Priority,
		})
		e.pool.Submit(execCtx, func() {
			if qm := e.queueMetrics; qm != nil && qm.ClaimToStart != nil {
				qm.ClaimToStart.Record(execCtx, time.Since(claimedAt).Seconds())
			}
			addWorkerRunBreadcrumb(execCtx, "worker.dispatch", "run dispatch starting", &run, nil, nil)
			defer func() {
				if r := recover(); r != nil {
					telemetry.AddSentryBreadcrumb(execCtx, "worker.dispatch", "worker panic", map[string]any{
						"run_id":         run.ID,
						"job_id":         run.JobID,
						"project_id":     run.ProjectID,
						"attempt":        run.Attempt,
						"execution_mode": string(run.ExecutionMode),
					})
					hub := sentry.GetHubFromContext(execCtx)
					if hub == nil {
						hub = sentry.CurrentHub()
					}
					hub.WithScope(func(scope *sentry.Scope) {
						e.applyWorkerSentryScope(scope, &run, map[string]any{
							"execution_mode": string(run.ExecutionMode),
						})
						scope.SetLevel(sentry.LevelFatal)
						hub.Recover(r)
					})
					sentry.Flush(2 * time.Second)
					e.logger.Error("panic in executor goroutine", "run_id", run.ID, "panic", r)
					e.handleSystemFailure(execCtx, &run, fmt.Sprintf("panic: %v", r))
				}
			}()
			e.execute(execCtx, &run)
		})
	}
}

// memStatsSampleInterval bounds how often checkMemoryPressure performs a
// stop-the-world runtime.ReadMemStats. Between samples the last verdict is
// reused, so a fast (degraded) poll loop does not pause the world every cycle.
const memStatsSampleInterval = time.Second

// checkMemoryPressure returns true (and logs) when heap pressure exceeds the
// configured threshold. ReadMemStats is throttled to memStatsSampleInterval
// because it stops the world; between samples the cached verdict is returned.
func (e *Executor) checkMemoryPressure() bool {
	if e.memoryPressureThreshold <= 0 {
		return false
	}
	now := time.Now().UnixNano()
	if last := e.memStatsLastSample.Load(); last != 0 && now-last < int64(memStatsSampleInterval) {
		return e.memStatsPressure.Load()
	}
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	heapPct := float64(memStats.HeapAlloc) / float64(memStats.Sys) * 100
	pressured := heapPct > e.memoryPressureThreshold
	e.memStatsPressure.Store(pressured)
	e.memStatsLastSample.Store(now)
	if pressured {
		e.logger.Warn("memory pressure: skipping dequeue", "heap_pct", heapPct, "threshold", e.memoryPressureThreshold)
	}
	return pressured
}

// computeAvailable returns the number of runs that can be dequeued this cycle,
// bounded by pool availability, the adaptive concurrency limit, and the max batch size.
func (e *Executor) computeAvailable() int {
	available := e.pool.Available()
	if e.concurrencyLimit != nil {
		target := max(e.concurrencyLimit.CurrentLimit(), 1)
		adaptiveAvailable := target - e.pool.ActiveCount()
		if adaptiveAvailable < available {
			available = adaptiveAvailable
		}
	}
	if e.maxDequeueBatchSize > 0 && available > e.maxDequeueBatchSize {
		available = e.maxDequeueBatchSize
	}
	return available
}

// dequeueRuns fetches up to capacity runs from the queue.
// In fair-share mode it round-robins across partitions; otherwise it performs
// a two-pass dequeue: HTTP-eligible runs first, then worker-eligible runs.
func (e *Executor) dequeueRuns(ctx context.Context, capacity int) ([]domain.JobRun, error) {
	if len(e.partitionCycle) > 0 {
		return e.dequeueAcrossPartitions(ctx, capacity)
	}

	// Pass 1: HTTP-eligible runs (any replica can dispatch these).
	// Prefer fully-denormalized legacy dequeue when explicitly requested,
	// otherwise claim_table > two_phase > DequeueN.
	var runs []domain.JobRun
	var err error
	if e.useDenormalizedDequeue {
		if dq, ok := e.queue.(fullyDenormalizedDequeuer); ok {
			runs, err = dq.DequeueNFullyDenormalized(ctx, capacity)
		} else {
			runs, err = e.queue.DequeueN(ctx, capacity)
		}
	} else if cq, ok := e.queue.(claimTableDequeuer); ok {
		runs, err = cq.DequeueNClaim(ctx, capacity)
	} else if tp, ok := e.queue.(twoPhaseDequeuer); ok {
		runs, err = tp.DequeueNTwoPhase(ctx, capacity)
	} else {
		runs, err = e.queue.DequeueN(ctx, capacity)
	}
	if err != nil {
		return nil, err
	}

	// Pass 2: Worker-eligible runs — only attempt if this replica has
	// connected workers and capacity remains after the HTTP pass.
	runs = e.appendWorkerRuns(ctx, runs, capacity)
	return runs, nil
}

// appendWorkerRuns dequeues worker-mode runs and appends them to runs when
// connected workers are available and remaining capacity allows it.
func (e *Executor) appendWorkerRuns(ctx context.Context, runs []domain.JobRun, capacity int) []domain.JobRun {
	if e.queueSnapshotter == nil {
		return runs
	}
	workerQueues := e.queueSnapshotter.SnapshotWorkerQueues()
	if len(workerQueues) == 0 {
		return runs
	}
	remaining := capacity - len(runs)
	if remaining <= 0 {
		return runs
	}
	wq, ok := e.queue.(workerQueueDequeuer)
	if !ok {
		return runs
	}
	workerRuns, wErr := wq.DequeueNForWorkerQueues(ctx, remaining, workerQueues)
	if wErr != nil {
		// Log but don't block the HTTP pass result.
		e.logger.Warn("worker dequeue failed", "error", wErr)
		return runs
	}
	return append(runs, workerRuns...)
}

func (e *Executor) dequeueAcrossPartitions(ctx context.Context, capacity int) ([]domain.JobRun, error) {
	out := make([]domain.JobRun, 0, capacity)
	if capacity <= 0 || len(e.partitionCycle) == 0 {
		return out, nil
	}

	if dq, ok := e.queue.(partitionedDequeuer); ok {
		qm, _ := queue.Metrics()
		partStart := time.Now()
		runs, err := dq.DequeueNPartitioned(ctx, capacity, e.partitionCycle)
		if qm != nil {
			qm.PartitionDequeueLag.Record(ctx, time.Since(partStart).Seconds())
		}
		return runs, err
	}

	remaining := capacity
	iterations := len(e.partitionCycle)
	qm, _ := queue.Metrics()
	for i := 0; i < iterations && remaining > 0; i++ {
		partition := e.partitionCycle[e.nextPartition%len(e.partitionCycle)]
		e.nextPartition = (e.nextPartition + 1) % len(e.partitionCycle)

		partStart := time.Now()
		claimed, err := e.queue.DequeueNByProject(ctx, remaining, partition)
		if qm != nil {
			// Avoid attaching partition/project_id as a label here;
			// in fair-share mode that would explode Prometheus cardinality.
			qm.PartitionDequeueLag.Record(ctx, time.Since(partStart).Seconds())
		}
		if err != nil {
			return nil, err
		}
		if len(claimed) == 0 {
			continue
		}

		out = append(out, claimed...)
		remaining -= len(claimed)
	}

	return out, nil
}

func buildPartitionCycle(partitions []string, weightsRaw string) []string {
	if len(partitions) == 0 {
		return nil
	}

	weights := make(map[string]int)
	if weightsRaw != "" {
		for _, token := range strings.FieldsFunc(weightsRaw, func(r rune) bool { return r == ',' }) {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			parts := strings.SplitN(token, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			weight, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || weight <= 0 {
				continue
			}
			weights[key] = weight
		}
	}

	cycle := make([]string, 0, len(partitions))
	for _, partition := range partitions {
		w := weights[partition]
		if w <= 0 {
			w = 1
		}
		for range w {
			cycle = append(cycle, partition)
		}
	}

	return cycle
}
