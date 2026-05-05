//go:build integration

package worker_test

import (
	"context"
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

type staticQueueSnapshotter struct {
	queues []string
}

func (s staticQueueSnapshotter) SnapshotQueues() []string {
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
		QueueSnapshotter:    staticQueueSnapshotter{queues: []string{"default"}},
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
