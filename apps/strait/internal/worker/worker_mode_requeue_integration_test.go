//go:build integration

package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	workergrpc "strait/internal/api/grpc"
	"strait/internal/domain"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/worker"
)

type noWorkerAvailableDispatcher struct{}

func (noWorkerAvailableDispatcher) WorkerDispatch(context.Context, *domain.JobRun, *domain.Job) (any, error) {
	return nil, errors.Join(errors.New("dispatcher unavailable"), workergrpc.ErrNoWorkerAvailable)
}

func (noWorkerAvailableDispatcher) ResultStatus(any) string { return "" }

func (noWorkerAvailableDispatcher) ResultError(any) string { return "" }

func (noWorkerAvailableDispatcher) ResultOutput(any) json.RawMessage { return nil }

type staticQueueSnapshotter struct {
	queues []domain.WorkerQueueRef
}

func (s staticQueueSnapshotter) SnapshotWorkerQueues() []domain.WorkerQueueRef {
	return s.queues
}

type successfulWorkerDispatcher struct {
	calls atomic.Int32
}

func (d *successfulWorkerDispatcher) WorkerDispatch(context.Context, *domain.JobRun, *domain.Job) (any, error) {
	d.calls.Add(1)
	return "ok", nil
}

func (*successfulWorkerDispatcher) ResultStatus(any) string { return "success" }

func (*successfulWorkerDispatcher) ResultError(any) string { return "" }

func (*successfulWorkerDispatcher) ResultOutput(any) json.RawMessage {
	return json.RawMessage(`{"ok":true}`)
}

type timeoutWorkerDispatcher struct {
	calls         atomic.Int32
	cancellations atomic.Int32
}

func (d *timeoutWorkerDispatcher) WorkerDispatch(ctx context.Context, _ *domain.JobRun, _ *domain.Job) (any, error) {
	d.calls.Add(1)
	<-ctx.Done()
	d.cancellations.Add(1)
	return nil, ctx.Err()
}

func (*timeoutWorkerDispatcher) ResultStatus(any) string { return "" }

func (*timeoutWorkerDispatcher) ResultError(any) string { return "" }

func (*timeoutWorkerDispatcher) ResultOutput(any) json.RawMessage { return nil }

func mustCreateWorkerModeJob(t *testing.T, ctx context.Context, st *store.Queries, projectID string) *domain.Job {
	t.Helper()

	job := &domain.Job{
		ID:            newID(),
		ProjectID:     projectID,
		Name:          "worker-job-" + newID(),
		Slug:          "worker-slug-" + newID(),
		MaxAttempts:   3,
		TimeoutSecs:   30,
		Enabled:       true,
		ExecutionMode: domain.ExecutionModeWorker,
		Queue:         "default",
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	return job
}

func TestWorkerModePollClaimsAndDispatchesWithWorkerPlane(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	st := store.New(env.DB.Pool)
	q := queue.NewPostgresQueue(env.DB.Pool)
	job := mustCreateWorkerModeJob(t, ctx, st, "project-worker-mode-dispatch")

	run := &domain.JobRun{
		ID:            newID(),
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Priority:      10,
		ExecutionMode: domain.ExecutionModeWorker,
		QueueName:     "default",
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dispatcher := &successfulWorkerDispatcher{}
	pool := worker.NewPool(1)
	exec := worker.NewExecutor(worker.ExecutorConfig{
		Pool:                pool,
		Queue:               q,
		Store:               st,
		TxPool:              env.DB.Pool,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
		PollInterval:        50 * time.Millisecond,
		HeartbeatInterval:   50 * time.Millisecond,
		WebhookMaxAttempts:  1,
		MaxDequeueBatchSize: 1,
		QueueSnapshotter:    staticQueueSnapshotter{queues: []domain.WorkerQueueRef{{ProjectID: job.ProjectID, QueueName: "default"}}},
		WorkerDispatcher:    dispatcher,
	})
	t.Cleanup(func() {
		exec.CloseCache()
		_ = pool.Shutdown(context.Background())
	})

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go exec.Run(execCtx)

	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for worker-mode run to complete")
		default:
		}

		got, err := st.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if got.Status != domain.StatusCompleted {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if dispatcher.calls.Load() == 0 {
			t.Fatal("worker dispatcher was not called")
		}
		var result map[string]bool
		if err := json.Unmarshal(got.Result, &result); err != nil {
			t.Fatalf("worker result is not valid JSON: %s: %v", got.Result, err)
		}
		if !result["ok"] {
			t.Fatalf("worker result = %s, want persisted output_json", got.Result)
		}
		cancel()
		return
	}
}

func TestWorkerModeNoWorkerAvailableRequeuesWithCleanQueuedFields(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	st := store.New(env.DB.Pool)
	q := queue.NewPostgresQueue(env.DB.Pool)
	job := mustCreateWorkerModeJob(t, ctx, st, "project-worker-mode-requeue")

	run := &domain.JobRun{
		ID:            newID(),
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Priority:      1,
		ExecutionMode: domain.ExecutionModeWorker,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	pgQueue := queue.NewPostgresQueue(env.DB.Pool)
	pool := worker.NewPool(1)
	exec := worker.NewExecutor(worker.ExecutorConfig{
		Pool:                pool,
		Queue:               pgQueue,
		Store:               st,
		TxPool:              env.DB.Pool,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
		PollInterval:        50 * time.Millisecond,
		HeartbeatInterval:   50 * time.Millisecond,
		WebhookMaxAttempts:  1,
		MaxDequeueBatchSize: 1,
		WorkerDispatcher:    noWorkerAvailableDispatcher{},
	})
	t.Cleanup(func() {
		exec.CloseCache()
		_ = pool.Shutdown(context.Background())
	})

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go exec.Run(execCtx)

	deadline := time.After(10 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for worker-mode run to requeue")
		default:
		}

		got, err := st.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if got.Status != domain.StatusQueued {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if got.StartedAt != nil || got.HeartbeatAt != nil || got.FinishedAt != nil || got.NextRetryAt != nil {
			t.Fatalf("queued run retained execution timestamps: started_at=%v heartbeat_at=%v finished_at=%v next_retry_at=%v", got.StartedAt, got.HeartbeatAt, got.FinishedAt, got.NextRetryAt)
		}
		if got.Error != "" || got.ErrorClass != "" {
			t.Fatalf("queued run retained error fields: error=%q error_class=%q", got.Error, got.ErrorClass)
		}
		cancel()
		return
	}
}

func TestWorkerModeDispatchHonorsJobTimeoutAndRequeues(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	st := store.New(env.DB.Pool)
	q := queue.NewPostgresQueue(env.DB.Pool)
	job := mustCreateWorkerModeJob(t, ctx, st, "project-worker-mode-timeout")
	job.TimeoutSecs = 1
	if err := st.UpdateJob(ctx, job); err != nil {
		t.Fatalf("UpdateJob(timeout_secs) error = %v", err)
	}

	run := &domain.JobRun{
		ID:            newID(),
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Priority:      1,
		ExecutionMode: domain.ExecutionModeWorker,
		QueueName:     "default",
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	dispatcher := &timeoutWorkerDispatcher{}
	pool := worker.NewPool(1)
	exec := worker.NewExecutor(worker.ExecutorConfig{
		Pool:                pool,
		Queue:               q,
		Store:               st,
		TxPool:              env.DB.Pool,
		HTTPClient:          &http.Client{Timeout: 5 * time.Second},
		PollInterval:        50 * time.Millisecond,
		HeartbeatInterval:   50 * time.Millisecond,
		WebhookMaxAttempts:  1,
		MaxDequeueBatchSize: 1,
		QueueSnapshotter:    staticQueueSnapshotter{queues: []domain.WorkerQueueRef{{ProjectID: job.ProjectID, QueueName: "default"}}},
		WorkerDispatcher:    dispatcher,
	})
	t.Cleanup(func() {
		exec.CloseCache()
		_ = pool.Shutdown(context.Background())
	})

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go exec.Run(execCtx)

	deadline := time.After(8 * time.Second)
	lastStatus := domain.RunStatus("")
	lastAttempt := 0
	lastError := ""
	for {
		select {
		case <-deadline:
			t.Fatalf(
				"timed out waiting for worker-mode timeout requeue; last status/attempt/error = %q/%d/%q",
				lastStatus,
				lastAttempt,
				lastError,
			)
		default:
		}

		got, err := st.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		lastStatus = got.Status
		lastAttempt = got.Attempt
		lastError = got.Error
		if got.Status == domain.StatusQueued && got.Attempt == 2 {
			if got.Error != "execution timed out" {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			if dispatcher.calls.Load() == 0 {
				t.Fatal("worker dispatcher was not called")
			}
			if dispatcher.cancellations.Load() == 0 {
				t.Fatal("worker dispatch context was not cancelled by timeout")
			}
			// Retry schedule lives in the job_retries side table now;
			// job_runs.next_retry_at is no longer written by the worker.
			var nextRetryAt *time.Time
			if err := env.DB.Pool.QueryRow(ctx,
				`SELECT next_retry_at
				 FROM job_retries
				 WHERE run_id = $1 AND cleared = FALSE
				 ORDER BY id DESC
				 LIMIT 1`, run.ID,
			).Scan(&nextRetryAt); err != nil {
				t.Fatalf("timed-out worker run missing job_retries row: %v", err)
			}
			if nextRetryAt == nil {
				t.Fatal("timed-out worker run was requeued without next_retry_at in job_retries")
			}
			cancel()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
