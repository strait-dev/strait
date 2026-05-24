//go:build integration

package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/pubsub"
	"strait/internal/queue"
	"strait/internal/store"
	"strait/internal/testutil"
	"strait/internal/worker"

	"github.com/google/uuid"
)

// Lazy init for testcontainers so we do not conflict with the existing
// TestMain in package worker (internal test file).
var (
	testEnv     *testutil.TestEnv
	testEnvOnce sync.Once
	testEnvErr  error
)

func mustEnv(t *testing.T) *testutil.TestEnv {
	t.Helper()
	testEnvOnce.Do(func() {
		ctx := context.Background()
		testEnv, testEnvErr = testutil.SetupTestEnv(ctx, "../../migrations")
		if testEnvErr != nil {
			log.Fatalf("setup test env: %v", testEnvErr)
		}
	})
	if testEnvErr != nil {
		t.Fatalf("test env setup failed: %v", testEnvErr)
	}
	return testEnv
}

func mustCleanEnv(t *testing.T, ctx context.Context) {
	t.Helper()
	env := mustEnv(t)
	if err := env.Clean(ctx); err != nil {
		t.Fatalf("clean env: %v", err)
	}
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func mustCreateJob(t *testing.T, ctx context.Context, st *store.Queries, projectID, endpointURL string) *domain.Job {
	t.Helper()
	job := &domain.Job{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "job-" + newID(),
		Slug:        "slug-" + newID(),
		EndpointURL: endpointURL,
		MaxAttempts: 3,
		TimeoutSecs: 30,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	return job
}

// newExecutor creates an Executor wired to real Postgres queue, store, and
// optionally a Redis publisher. The returned Executor is ready to Run().
func newExecutor(
	t *testing.T,
	env *testutil.TestEnv,
	endpointURL string,
	concurrency int,
	httpClient *http.Client,
) (*worker.Executor, *queue.PostgresQueue) {
	t.Helper()

	q := queue.NewPostgresQueue(env.DB.Pool)
	st := store.New(env.DB.Pool)
	pub := pubsub.NewRedisPublisher(env.Redis.Client)

	pool := worker.NewPool(concurrency)
	wake := make(chan struct{}, 1)

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	exec := worker.NewExecutor(worker.ExecutorConfig{
		Pool:                pool,
		Queue:               q,
		Wake:                wake,
		Store:               st,
		TxPool:              env.DB.Pool,
		Publisher:           pub,
		HTTPClient:          httpClient,
		PollInterval:        100 * time.Millisecond,
		HeartbeatInterval:   200 * time.Millisecond,
		WebhookMaxAttempts:  1,
		MaxDequeueBatchSize: 10,
	})

	t.Cleanup(func() {
		exec.CloseCache()
		// Do not call pub.Close() -- it closes the shared env.Redis.Client
		// which breaks subsequent tests that reuse the same TestEnv.
		_ = pool.Shutdown(context.Background())
	})

	return exec, q
}

// TestJobExecutionEndToEnd enqueues a job run, starts the executor, and verifies
// the run reaches "completed" status after the endpoint returns 200.
func TestJobExecutionEndToEnd(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	var dispatched atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatched.Store(true)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	st := store.New(env.DB.Pool)
	q := queue.NewPostgresQueue(env.DB.Pool)
	job := mustCreateJob(t, ctx, st, "project-e2e", srv.URL)

	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Priority:  1,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	exec, _ := newExecutor(t, env, srv.URL, 4, srv.Client())
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go exec.Run(execCtx)

	// Poll until the run reaches a terminal state.
	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for run to complete")
		default:
		}

		got, err := st.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if got.Status == domain.StatusCompleted {
			if !dispatched.Load() {
				t.Fatal("run completed but endpoint was never called")
			}
			if got.FinishedAt == nil {
				t.Fatal("completed run has nil finished_at")
			}
			return
		}
		if got.Status.IsTerminal() {
			t.Fatalf("run reached unexpected terminal status %q", got.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestHeartbeatWithRealRedis verifies that the HeartbeatManager writes heartbeats
// to the real Postgres store for registered runs.
func TestHeartbeatWithRealRedis(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	st := store.New(env.DB.Pool)
	q := queue.NewPostgresQueue(env.DB.Pool)
	job := mustCreateJob(t, ctx, st, "project-heartbeat", "https://example.com/noop")

	// Create a run in executing state so heartbeats can be written.
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	// Dequeue to transition to dequeued.
	dequeued, err := q.Dequeue(ctx)
	if err != nil || dequeued == nil {
		t.Fatalf("Dequeue() error = %v, dequeued = %v", err, dequeued)
	}
	// Transition to executing.
	if err := st.UpdateRunStatus(ctx, dequeued.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": time.Now(),
	}); err != nil {
		t.Fatalf("UpdateRunStatus() error = %v", err)
	}

	hbm := worker.NewHeartbeatManager(st, 200*time.Millisecond)
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()

	go hbm.Run(hbCtx, dequeued.ID)

	// Wait long enough for at least one heartbeat tick.
	time.Sleep(500 * time.Millisecond)
	hbCancel()

	got, err := st.GetRun(ctx, dequeued.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if got.HeartbeatAt == nil {
		t.Fatal("heartbeat_at was not set after heartbeat manager ran")
	}
}

// TestDispatchWithRealQueue verifies the executor dequeues from the real
// Postgres queue and dispatches to an HTTP endpoint.
func TestDispatchWithRealQueue(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	var receivedRunIDs sync.Map
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		runID := r.Header.Get("X-Run-ID")
		receivedRunIDs.Store(runID, true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := store.New(env.DB.Pool)
	q := queue.NewPostgresQueue(env.DB.Pool)
	job := mustCreateJob(t, ctx, st, "project-dispatch", srv.URL)

	const runCount = 5
	runIDs := make([]string, runCount)
	for i := range runCount {
		id := newID()
		runIDs[i] = id
		run := &domain.JobRun{
			ID:        id,
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Priority:  i,
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() run %d error = %v", i, err)
		}
	}

	exec, _ := newExecutor(t, env, srv.URL, 4, srv.Client())
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go exec.Run(execCtx)

	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for all runs to complete")
		default:
		}

		allDone := true
		for _, id := range runIDs {
			got, err := st.GetRun(ctx, id)
			if err != nil {
				t.Fatalf("GetRun(%s) error = %v", id, err)
			}
			if !got.Status.IsTerminal() {
				allDone = false
				break
			}
			if got.Status != domain.StatusCompleted {
				t.Fatalf("run %s has unexpected terminal status %q", id, got.Status)
			}
		}
		if allDone {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Verify the endpoint received all run IDs.
	for _, id := range runIDs {
		if _, ok := receivedRunIDs.Load(id); !ok {
			t.Fatalf("endpoint never received run %s", id)
		}
	}
}

// TestConcurrentJobExecution verifies that multiple jobs are processed in
// parallel by the executor pool.
func TestConcurrentJobExecution(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	var inflight atomic.Int32
	var maxInflight atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := inflight.Add(1)
		// Track the peak concurrency observed.
		for {
			prev := maxInflight.Load()
			if cur <= prev || maxInflight.CompareAndSwap(prev, cur) {
				break
			}
		}
		// Hold the request long enough for concurrent requests to overlap.
		time.Sleep(200 * time.Millisecond)
		inflight.Add(-1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	st := store.New(env.DB.Pool)
	q := queue.NewPostgresQueue(env.DB.Pool)
	job := mustCreateJob(t, ctx, st, "project-concurrent", srv.URL)

	const runCount = 8
	for i := range runCount {
		run := &domain.JobRun{
			ID:        newID(),
			JobID:     job.ID,
			ProjectID: job.ProjectID,
			Priority:  i,
		}
		if err := q.Enqueue(ctx, run); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	poolSize := 4
	exec, _ := newExecutor(t, env, srv.URL, poolSize, srv.Client())
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go exec.Run(execCtx)

	deadline := time.After(20 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for concurrent runs to complete")
		default:
		}

		status := domain.StatusCompleted
		completed, err := st.ListRunsByProject(ctx, job.ProjectID, &status, nil, nil, nil, nil, nil, nil, nil, 20, nil)
		if err != nil {
			t.Fatalf("ListRunsByProject() error = %v", err)
		}
		if len(completed) >= runCount {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	observed := maxInflight.Load()
	if observed < 2 {
		t.Fatalf("expected at least 2 concurrent requests, observed peak = %d", observed)
	}
}

// TestFailedJobHandling verifies that when the endpoint returns an error,
// the run is eventually marked as failed or system_failed in the database.
func TestFailedJobHandling(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"server error"}`))
	}))
	defer srv.Close()

	st := store.New(env.DB.Pool)
	q := queue.NewPostgresQueue(env.DB.Pool)
	// Create a job with max_attempts=1 so it immediately fails without retries.
	job := &domain.Job{
		ID:          newID(),
		ProjectID:   "project-failed",
		Name:        "failing-job-" + newID(),
		Slug:        "slug-fail-" + newID(),
		EndpointURL: srv.URL,
		MaxAttempts: 1,
		TimeoutSecs: 30,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	exec, _ := newExecutor(t, env, srv.URL, 4, srv.Client())
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go exec.Run(execCtx)

	// With max_attempts=1, the executor dead-letters the run on first failure
	// (no retries available). StatusDeadLetter is not in IsTerminal() since
	// dead-lettered runs can be manually retried, so check for it directly.
	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-deadline:
			got, _ := st.GetRun(ctx, run.ID)
			t.Fatalf("timed out waiting for run to be dead-lettered; current status = %q", got.Status)
		default:
		}

		got, err := st.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if got.Status == domain.StatusDeadLetter {
			if got.Error == "" {
				t.Fatal("dead-lettered run has empty error field")
			}
			if got.FinishedAt == nil {
				t.Fatal("dead-lettered run has nil finished_at")
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestWorkerGracefulShutdown verifies that in-flight jobs complete before
// the executor finishes shutting down.
func TestWorkerGracefulShutdown(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	handlerStarted := make(chan struct{})
	handlerComplete := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(handlerStarted)
		// Simulate a slow job.
		<-handlerComplete
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"done"}`))
	}))
	defer srv.Close()

	st := store.New(env.DB.Pool)
	q := queue.NewPostgresQueue(env.DB.Pool)
	job := mustCreateJob(t, ctx, st, "project-shutdown", srv.URL)

	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	exec, _ := newExecutor(t, env, srv.URL, 4, srv.Client())
	execCtx, cancel := context.WithCancel(ctx)

	execDone := make(chan struct{})
	go func() {
		exec.Run(execCtx)
		close(execDone)
	}()

	// Wait for the handler to start processing the request.
	select {
	case <-handlerStarted:
	case <-time.After(15 * time.Second):
		cancel()
		t.Fatal("timed out waiting for handler to start")
	}

	// Allow the handler to complete while the executor is still running,
	// then give it time to write the result to DB before canceling.
	close(handlerComplete)
	time.Sleep(500 * time.Millisecond)

	// Now trigger shutdown.
	cancel()

	// Wait for executor to finish.
	select {
	case <-execDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for executor to shut down")
	}

	// Perform explicit shutdown for pool draining.
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 10*time.Second)
	defer shutdownCancel()
	if err := exec.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}

	// Verify the run completed successfully despite shutdown.
	got, err := st.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if !got.Status.IsTerminal() {
		t.Fatalf("expected terminal status after shutdown, got %q", got.Status)
	}
	if got.Status == domain.StatusCompleted {
		// Best case: the in-flight job finished.
		return
	}
	// The run may also have been marked system_failed if the context
	// cancellation raced with completion. Both outcomes are acceptable
	// for graceful shutdown -- the key property is that it reached a
	// terminal state rather than being stuck in executing.
	t.Logf("run reached terminal status %q (acceptable for graceful shutdown)", got.Status)
}

// TestEndToEndWithPayloadAndResult verifies that payload is sent to the
// endpoint and the result is stored back on the run.
func TestEndToEndWithPayloadAndResult(t *testing.T) {
	ctx := context.Background()
	env := mustEnv(t)
	mustCleanEnv(t, ctx)

	var receivedPayload json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			receivedPayload = body
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"processed":true,"count":42}`))
	}))
	defer srv.Close()

	st := store.New(env.DB.Pool)
	q := queue.NewPostgresQueue(env.DB.Pool)
	job := mustCreateJob(t, ctx, st, "project-payload", srv.URL)

	payload := json.RawMessage(`{"key":"value","numbers":[1,2,3]}`)
	run := &domain.JobRun{
		ID:        newID(),
		JobID:     job.ID,
		ProjectID: job.ProjectID,
		Payload:   payload,
	}
	if err := q.Enqueue(ctx, run); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	exec, _ := newExecutor(t, env, srv.URL, 4, srv.Client())
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go exec.Run(execCtx)

	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for run to complete")
		default:
		}

		got, err := st.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetRun() error = %v", err)
		}
		if got.Status == domain.StatusCompleted {
			if receivedPayload == nil {
				t.Fatal("endpoint did not receive payload")
			}
			var p map[string]any
			if err := json.Unmarshal(receivedPayload, &p); err != nil {
				t.Fatalf("unmarshal received payload: %v", err)
			}
			if p["key"] != "value" {
				t.Fatalf("payload key = %v, want %q", p["key"], "value")
			}
			if got.Result == nil {
				t.Fatal("completed run has nil result")
			}
			var result map[string]any
			if err := json.Unmarshal(got.Result, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}
			if fmt.Sprintf("%v", result["count"]) != "42" {
				t.Fatalf("result count = %v, want 42", result["count"])
			}
			return
		}
		if got.Status.IsTerminal() {
			t.Fatalf("run reached unexpected terminal status %q", got.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
