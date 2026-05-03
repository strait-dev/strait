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

	"github.com/getsentry/sentry-go"
)

// twoPhaseDequeuer claims IDs first, then fetches full rows by PK.
type twoPhaseDequeuer interface {
	DequeueNTwoPhase(ctx context.Context, n int) ([]domain.JobRun, error)
}

// claimTableDequeuer deletes from job_run_queue, then fetches from job_runs.
type claimTableDequeuer interface {
	DequeueNClaim(ctx context.Context, n int) ([]domain.JobRun, error)
}

// workerQueueDequeuer claims worker-mode runs for specific queues.
type workerQueueDequeuer interface {
	DequeueNForWorker(ctx context.Context, n int, queues []string) ([]domain.JobRun, error)
}

// QueueSnapshotter returns the set of queue names that have active workers
// connected to this replica. Implemented by grpc.ConnectionRegistry via
// an adapter to avoid a circular import.
type QueueSnapshotter interface {
	SnapshotQueues() []string
}

func (e *Executor) poll(ctx context.Context) {
	start := time.Now()
	if e.memoryPressureThreshold > 0 {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		heapPct := float64(memStats.HeapAlloc) / float64(memStats.Sys) * 100
		if heapPct > e.memoryPressureThreshold {
			e.logger.Warn("memory pressure: skipping dequeue", "heap_pct", heapPct, "threshold", e.memoryPressureThreshold)
			return
		}
	}
	available := e.pool.Available()
	if e.concurrencyLimit != nil {
		target := max(e.concurrencyLimit.CurrentLimit(), 1)
		adaptiveAvailable := target - e.pool.ActiveCount()
		if adaptiveAvailable < available {
			available = adaptiveAvailable
		}
	}
	if available <= 0 {
		return
	}
	if e.maxDequeueBatchSize > 0 && available > e.maxDequeueBatchSize {
		available = e.maxDequeueBatchSize
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
		switch {
		case len(e.partitionCycle) > 0:
			runs, err = e.dequeueAcrossPartitions(innerCtx, available)
		default:
			// Pass 1: HTTP-eligible runs (any replica can dispatch these).
			// Prefer claim_table > two_phase > DequeueN.
			if cq, ok := e.queue.(claimTableDequeuer); ok {
				runs, err = cq.DequeueNClaim(innerCtx, available)
			} else if tp, ok := e.queue.(twoPhaseDequeuer); ok {
				runs, err = tp.DequeueNTwoPhase(innerCtx, available)
			} else {
				runs, err = e.queue.DequeueN(innerCtx, available)
			}
			if err != nil {
				return err
			}

			// Pass 2: Worker-eligible runs — only attempt if this replica has
			// connected workers and capacity remains after the HTTP pass.
			if e.queueSnapshotter != nil {
				workerQueues := e.queueSnapshotter.SnapshotQueues()
				if len(workerQueues) > 0 {
					remaining := available - len(runs)
					if remaining > 0 {
						if wq, ok := e.queue.(workerQueueDequeuer); ok {
							workerRuns, wErr := wq.DequeueNForWorker(innerCtx, remaining, workerQueues)
							if wErr != nil {
								// Log but don't block the HTTP pass result.
								e.logger.Warn("worker dequeue failed", "error", wErr)
							} else {
								runs = append(runs, workerRuns...)
							}
						}
					}
				}
			}
		}
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

		execCtx := withDispatchCache(context.WithoutCancel(ctx))
		e.pool.Submit(execCtx, func() {
			if qm, qmErr := queue.Metrics(); qmErr == nil && qm != nil {
				qm.ClaimToStart.Record(execCtx, time.Since(claimedAt).Seconds())
			}
			defer func() {
				if r := recover(); r != nil {
					sentry.WithScope(func(scope *sentry.Scope) {
						scope.SetTag("run_id", run.ID)
						scope.SetTag("job_id", run.JobID)
						scope.SetTag("project_id", run.ProjectID)
						scope.SetTag("attempt", fmt.Sprintf("%d", run.Attempt))
						scope.SetTag("execution_mode", string(run.ExecutionMode))
						scope.SetLevel(sentry.LevelFatal)
						scope.SetContext("run", map[string]any{
							"run_id":         run.ID,
							"job_id":         run.JobID,
							"project_id":     run.ProjectID,
							"attempt":        run.Attempt,
							"priority":       run.Priority,
							"execution_mode": run.ExecutionMode,
							"status":         run.Status,
						})
						sentry.CurrentHub().Recover(r)
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

func (e *Executor) dequeueAcrossPartitions(ctx context.Context, capacity int) ([]domain.JobRun, error) {
	out := make([]domain.JobRun, 0, capacity)
	if capacity <= 0 || len(e.partitionCycle) == 0 {
		return out, nil
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
