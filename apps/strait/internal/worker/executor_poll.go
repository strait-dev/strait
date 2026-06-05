package worker

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/telemetry"

	"github.com/getsentry/sentry-go"
)

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
