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

// workerQueueDequeuer claims worker-mode runs for specific queues.
type workerQueueDequeuer interface {
	DequeueNForWorkerQueues(ctx context.Context, n int, queues []domain.WorkerQueueRef) ([]domain.JobRun, error)
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
			if qm, qmErr := queue.Metrics(); qmErr == nil && qm != nil {
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

// checkMemoryPressure returns true (and logs) when heap pressure exceeds the configured threshold.
func (e *Executor) checkMemoryPressure() bool {
	if e.memoryPressureThreshold <= 0 {
		return false
	}
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	heapPct := float64(memStats.HeapAlloc) / float64(memStats.Sys) * 100
	if heapPct > e.memoryPressureThreshold {
		e.logger.Warn("memory pressure: skipping dequeue", "heap_pct", heapPct, "threshold", e.memoryPressureThreshold)
		return true
	}
	return false
}

// computeAvailable returns the number of runs that can be dequeued this cycle,
// bounded by pool availability, the adaptive concurrency limit, and the max batch size.
func (e *Executor) computeAvailable() int {
	active, available := e.pool.observedSnapshot()
	if e.concurrencyLimit != nil {
		target := max(e.concurrencyLimit.CurrentLimit(), 1)
		adaptiveAvailable := target - active
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
	claimed := newClaimedRunBatch(capacity)
	runs, err := e.queue.DequeueN(ctx, claimed.remaining())
	if err != nil {
		return nil, err
	}
	claimed.append(runs)

	// Pass 2: Worker-eligible runs — only attempt if this replica has
	// connected workers and capacity remains after the HTTP pass.
	e.appendWorkerRuns(ctx, &claimed)
	return claimed.runs, nil
}

// appendWorkerRuns dequeues worker-mode runs and appends them to runs when
// connected workers are available and remaining capacity allows it.
func (e *Executor) appendWorkerRuns(ctx context.Context, claimed *claimedRunBatch) {
	if e.queueSnapshotter == nil {
		return
	}
	workerQueues := e.queueSnapshotter.SnapshotWorkerQueues()
	if len(workerQueues) == 0 {
		return
	}
	wq, ok := e.queue.(workerQueueDequeuer)
	if !ok || claimed.full() {
		return
	}
	workerRuns, wErr := wq.DequeueNForWorkerQueues(ctx, claimed.remaining(), workerQueues)
	if wErr != nil {
		// Log but don't block the HTTP pass result.
		e.logger.Warn("worker dequeue failed", "error", wErr)
		return
	}
	claimed.append(workerRuns)
}

func (e *Executor) dequeueAcrossPartitions(ctx context.Context, capacity int) ([]domain.JobRun, error) {
	claimed := newClaimedRunBatch(capacity)
	if capacity <= 0 || len(e.partitionCycle) == 0 {
		return claimed.runs, nil
	}

	iterations := len(e.partitionCycle)
	qm, _ := queue.Metrics()
	for i := 0; i < iterations && !claimed.full(); i++ {
		partition := e.partitionCycle[e.nextPartition%len(e.partitionCycle)]
		e.nextPartition = (e.nextPartition + 1) % len(e.partitionCycle)

		partStart := time.Now()
		runs, err := e.queue.DequeueNByProject(ctx, claimed.remaining(), partition)
		if qm != nil {
			// Avoid attaching partition/project_id as a label here;
			// in fair-share mode that would explode Prometheus cardinality.
			qm.PartitionDequeueLag.Record(ctx, time.Since(partStart).Seconds())
		}
		if err != nil {
			return nil, err
		}
		if len(runs) == 0 {
			continue
		}

		claimed.append(runs)
	}

	return claimed.runs, nil
}

type claimedRunBatch struct {
	runs     []domain.JobRun
	capacity int
}

func newClaimedRunBatch(capacity int) claimedRunBatch {
	if capacity < 0 {
		capacity = 0
	}
	return claimedRunBatch{
		runs:     make([]domain.JobRun, 0, capacity),
		capacity: capacity,
	}
}

func (b *claimedRunBatch) append(runs []domain.JobRun) {
	b.runs = append(b.runs, runs...)
}

func (b *claimedRunBatch) full() bool {
	return b.remaining() <= 0
}

func (b *claimedRunBatch) remaining() int {
	return max(b.capacity-len(b.runs), 0)
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
