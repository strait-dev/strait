package worker

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"time"

	"strait/internal/domain"

	"github.com/getsentry/sentry-go"
)

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

	var runs []domain.JobRun
	var err error
	if len(e.partitionCycle) > 0 {
		runs, err = e.dequeueAcrossPartitions(ctx, available)
	} else if e.dequeueStrategy == "fair_round_robin" {
		runs, err = e.queue.DequeueNFair(ctx, available)
	} else {
		runs, err = e.queue.DequeueN(ctx, available)
	}
	if e.metrics != nil {
		e.metrics.DequeueDuration.Record(ctx, time.Since(start).Seconds())
	}
	if err != nil {
		e.logger.Error("dequeue failed", "error", err)
		return
	}
	if len(runs) == 0 {
		return
	}

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

		execCtx := context.WithoutCancel(ctx)
		e.pool.Submit(execCtx, func() {
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
	for i := 0; i < iterations && remaining > 0; i++ {
		partition := e.partitionCycle[e.nextPartition%len(e.partitionCycle)]
		e.nextPartition = (e.nextPartition + 1) % len(e.partitionCycle)

		claimed, err := e.queue.DequeueNByProject(ctx, remaining, partition)
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
		for i := 0; i < w; i++ {
			cycle = append(cycle, partition)
		}
	}

	return cycle
}
