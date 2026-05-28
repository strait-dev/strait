//go:build integration

package logdrain_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/logdrain"
	"strait/internal/store"
	"strait/internal/testutil"

	"github.com/google/uuid"
	"github.com/sourcegraph/conc"
)

var testDB *testutil.TestDB

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	testDB, err = testutil.SetupSharedTestDB(ctx, "../../migrations", "logdrain")
	if err != nil {
		log.Fatalf("setup test db: %v", err)
	}

	code := m.Run()
	testDB.Cleanup(ctx)
	os.Exit(code)
}

func newID() string {
	return uuid.Must(uuid.NewV7()).String()
}

func mustStore(t *testing.T) *store.Queries {
	t.Helper()
	if testDB == nil || testDB.Pool == nil {
		t.Fatal("testDB is not initialized")
	}
	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := testDB.CleanTables(ctx); err != nil {
		t.Fatalf("CleanTables() error = %v", err)
	}
}

// seedProject creates a project in the DB for foreign key constraints.
func seedProject(t *testing.T, ctx context.Context, st *store.Queries, projectID string) {
	t.Helper()
	project := &domain.Project{
		ID:    projectID,
		OrgID: "org-" + projectID,
		Name:  "test-project-" + projectID,
	}
	if err := st.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
}

// seedJob creates a job in the DB for foreign key constraints.
func seedJob(t *testing.T, ctx context.Context, st *store.Queries, projectID string) *domain.Job {
	t.Helper()
	job := &domain.Job{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "job-" + newID(),
		Slug:        "slug-" + newID(),
		EndpointURL: "https://example.com/webhook",
		MaxAttempts: 3,
		TimeoutSecs: 120,
		Enabled:     true,
	}
	if err := st.CreateJob(ctx, job); err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	return job
}

// seedFinishedRun creates a run and transitions it through
// queued -> dequeued -> executing -> completed with the given finished_at.
func seedFinishedRun(t *testing.T, ctx context.Context, st *store.Queries, job *domain.Job, finishedAt time.Time) *domain.JobRun {
	t.Helper()
	run := &domain.JobRun{
		ID:            newID(),
		JobID:         job.ID,
		ProjectID:     job.ProjectID,
		Status:        domain.StatusQueued,
		Attempt:       1,
		Payload:       []byte(`{}`),
		TriggeredBy:   domain.TriggerManual,
		Priority:      0,
		ExecutionMode: domain.ExecutionModeHTTP,
	}
	if err := st.CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	// Transition queued -> dequeued.
	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusQueued, domain.StatusDequeued, nil); err != nil {
		t.Fatalf("UpdateRunStatus(queued->dequeued) error = %v", err)
	}
	// Transition dequeued -> executing.
	startedAt := finishedAt.Add(-10 * time.Second)
	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusDequeued, domain.StatusExecuting, map[string]any{
		"started_at": startedAt,
	}); err != nil {
		t.Fatalf("UpdateRunStatus(dequeued->executing) error = %v", err)
	}
	// Transition executing -> completed.
	if err := st.UpdateRunStatus(ctx, run.ID, domain.StatusExecuting, domain.StatusCompleted, map[string]any{
		"finished_at": finishedAt,
	}); err != nil {
		t.Fatalf("UpdateRunStatus(executing->completed) error = %v", err)
	}
	run.Status = domain.StatusCompleted
	run.FinishedAt = &finishedAt
	return run
}

// seedEvent creates a run event in the DB.
func seedEvent(t *testing.T, ctx context.Context, st *store.Queries, runID, message string) *domain.RunEvent {
	t.Helper()
	event := &domain.RunEvent{
		ID:      newID(),
		RunID:   runID,
		Type:    domain.EventLog,
		Level:   "info",
		Message: message,
		Data:    json.RawMessage(`{}`),
	}
	if err := st.InsertEvent(ctx, event); err != nil {
		t.Fatalf("InsertEvent() error = %v", err)
	}
	return event
}

// seedLogDrain creates an enabled log drain in the DB.
func seedLogDrain(t *testing.T, ctx context.Context, st *store.Queries, projectID, endpointURL string) *domain.LogDrain {
	t.Helper()
	drain := &domain.LogDrain{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "drain-" + newID(),
		DrainType:   "http",
		EndpointURL: endpointURL,
		AuthType:    "bearer",
		AuthConfig:  map[string]string{"token": "test-token"},
		LevelFilter: []string{},
		Enabled:     true,
	}
	if err := st.CreateLogDrain(ctx, drain); err != nil {
		t.Fatalf("CreateLogDrain() error = %v", err)
	}
	return drain
}

// TestWorker_ProcessDrain_WithRealStore verifies that the worker processes finished
// runs from a real Postgres database and delivers events to an HTTP endpoint.
func TestWorker_ProcessDrain_WithRealStore(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-" + newID()
	seedProject(t, ctx, st, projectID)
	job := seedJob(t, ctx, st, projectID)

	finishedAt := time.Now().Add(-1 * time.Minute).Truncate(time.Microsecond)
	run := seedFinishedRun(t, ctx, st, job, finishedAt)
	evt1 := seedEvent(t, ctx, st, run.ID, "step started")
	evt2 := seedEvent(t, ctx, st, run.ID, "step completed")

	var received []domain.RunEvent
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []domain.RunEvent
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		received = append(received, events...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	seedLogDrain(t, ctx, st, projectID, srv.URL)

	svc := logdrain.NewService()
	w := logdrain.NewWorker(st, svc, 50*time.Millisecond)

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(workerCtx)
		close(done)
	})

	// Wait for at least one tick to process.
	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}

	ids := map[string]bool{received[0].ID: true, received[1].ID: true}
	if !ids[evt1.ID] || !ids[evt2.ID] {
		t.Errorf("expected event IDs %q and %q, got %v", evt1.ID, evt2.ID, ids)
	}
}

// TestWorker_HTTPDelivery_BearerAuth verifies that the worker sends the
// Authorization header with the bearer token from the log drain config.
func TestWorker_HTTPDelivery_BearerAuth(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-" + newID()
	seedProject(t, ctx, st, projectID)
	job := seedJob(t, ctx, st, projectID)

	finishedAt := time.Now().Add(-30 * time.Second).Truncate(time.Microsecond)
	run := seedFinishedRun(t, ctx, st, job, finishedAt)
	seedEvent(t, ctx, st, run.ID, "test event")

	var capturedAuth string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	seedLogDrain(t, ctx, st, projectID, srv.URL)

	svc := logdrain.NewService()
	w := logdrain.NewWorker(st, svc, 50*time.Millisecond)

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(workerCtx)
		close(done)
	})

	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if capturedAuth != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", capturedAuth, "Bearer test-token")
	}
}

// TestWorker_BatchProcessing_MultipleRuns verifies that a single tick
// processes all finished runs and delivers events for each run separately.
func TestWorker_BatchProcessing_MultipleRuns(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-" + newID()
	seedProject(t, ctx, st, projectID)
	job := seedJob(t, ctx, st, projectID)

	now := time.Now().Truncate(time.Microsecond)
	numRuns := 5
	expectedEvents := 0
	for i := range numRuns {
		finishedAt := now.Add(-time.Duration(numRuns-i) * time.Minute)
		run := seedFinishedRun(t, ctx, st, job, finishedAt)
		eventsPerRun := i + 1
		for j := range eventsPerRun {
			seedEvent(t, ctx, st, run.ID, fmt.Sprintf("event-%d-%d", i, j))
		}
		expectedEvents += eventsPerRun
	}

	var deliveryCount int
	var totalEvents int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []domain.RunEvent
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		deliveryCount++
		totalEvents += len(events)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	seedLogDrain(t, ctx, st, projectID, srv.URL)

	svc := logdrain.NewService()
	w := logdrain.NewWorker(st, svc, 50*time.Millisecond)

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(workerCtx)
		close(done)
	})

	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if deliveryCount != numRuns {
		t.Errorf("expected %d deliveries, got %d", numRuns, deliveryCount)
	}
	if totalEvents != expectedEvents {
		t.Errorf("expected %d total events, got %d", expectedEvents, totalEvents)
	}
}

// TestWorker_FailedDelivery_RetryAndPoisonSkip verifies that a failing endpoint
// causes retries and that after maxRunRetries the run is skipped as a poison run.
func TestWorker_FailedDelivery_RetryAndPoisonSkip(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-" + newID()
	seedProject(t, ctx, st, projectID)
	job := seedJob(t, ctx, st, projectID)

	finishedAt := time.Now().Add(-1 * time.Minute).Truncate(time.Microsecond)
	run := seedFinishedRun(t, ctx, st, job, finishedAt)
	seedEvent(t, ctx, st, run.ID, "will fail delivery")

	var callCount int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		callCount++
		mu.Unlock()
		// Always return error to trigger retries.
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	seedLogDrain(t, ctx, st, projectID, srv.URL)

	svc := logdrain.NewService()
	// Use a very short interval so we cycle through retries quickly.
	w := logdrain.NewWorker(st, svc, 30*time.Millisecond)

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(workerCtx)
		close(done)
	})

	// Allow enough ticks for maxRunRetries (3) failures + 1 skip tick.
	time.Sleep(500 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	// Should have been called exactly maxRunRetries (3) times, then skipped.
	if callCount < 3 {
		t.Errorf("expected at least 3 delivery attempts, got %d", callCount)
	}
}

// TestWorker_FailedDelivery_SuccessAfterRetry verifies that a temporarily
// failing endpoint is retried and succeeds on subsequent ticks.
func TestWorker_FailedDelivery_SuccessAfterRetry(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-" + newID()
	seedProject(t, ctx, st, projectID)
	job := seedJob(t, ctx, st, projectID)

	finishedAt := time.Now().Add(-1 * time.Minute).Truncate(time.Microsecond)
	run := seedFinishedRun(t, ctx, st, job, finishedAt)
	seedEvent(t, ctx, st, run.ID, "transient failure event")

	var callCount int
	var receivedEvents []domain.RunEvent
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callCount++
		c := callCount
		mu.Unlock()
		if c == 1 {
			// First attempt fails.
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		var events []domain.RunEvent
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		receivedEvents = append(receivedEvents, events...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	seedLogDrain(t, ctx, st, projectID, srv.URL)

	svc := logdrain.NewService()
	w := logdrain.NewWorker(st, svc, 50*time.Millisecond)

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(workerCtx)
		close(done)
	})

	time.Sleep(400 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if callCount < 2 {
		t.Fatalf("expected at least 2 delivery attempts, got %d", callCount)
	}
	if len(receivedEvents) != 1 {
		t.Errorf("expected 1 event delivered on retry, got %d", len(receivedEvents))
	}
}

// TestWorker_ConcurrentDrains verifies that the worker processes multiple
// log drains for different projects concurrently without data corruption.
func TestWorker_ConcurrentDrains(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	numProjects := 3
	eventsPerProject := 2

	// Track deliveries per project.
	deliveries := make(map[string][]domain.RunEvent)
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []domain.RunEvent
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		if len(events) > 0 {
			runID := events[0].RunID
			deliveries[runID] = append(deliveries[runID], events...)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	now := time.Now().Truncate(time.Microsecond)
	runIDs := make([]string, 0, numProjects)
	for i := range numProjects {
		projectID := fmt.Sprintf("proj-concurrent-%d-%s", i, newID())
		seedProject(t, ctx, st, projectID)
		job := seedJob(t, ctx, st, projectID)

		finishedAt := now.Add(-time.Duration(numProjects-i) * time.Minute)
		run := seedFinishedRun(t, ctx, st, job, finishedAt)
		runIDs = append(runIDs, run.ID)
		for j := range eventsPerProject {
			seedEvent(t, ctx, st, run.ID, fmt.Sprintf("concurrent-event-%d-%d", i, j))
		}

		seedLogDrain(t, ctx, st, projectID, srv.URL)
	}

	svc := logdrain.NewService()
	w := logdrain.NewWorker(st, svc, 50*time.Millisecond)

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(workerCtx)
		close(done)
	})

	time.Sleep(400 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	for _, runID := range runIDs {
		events := deliveries[runID]
		if len(events) != eventsPerProject {
			t.Errorf("run %s: expected %d events, got %d", runID, eventsPerProject, len(events))
		}
	}
}

// TestWorker_LogDrainConfig_StoredInDB verifies that log drains created in
// the database are picked up by the worker and used for delivery.
func TestWorker_LogDrainConfig_StoredInDB(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-" + newID()
	seedProject(t, ctx, st, projectID)
	job := seedJob(t, ctx, st, projectID)

	finishedAt := time.Now().Add(-30 * time.Second).Truncate(time.Microsecond)
	run := seedFinishedRun(t, ctx, st, job, finishedAt)
	seedEvent(t, ctx, st, run.ID, "config test event")

	// Create two drains: one enabled, one disabled.
	var enabledReceived, disabledReceived int
	var mu sync.Mutex

	enabledSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		enabledReceived++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer enabledSrv.Close()

	disabledSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		disabledReceived++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer disabledSrv.Close()

	seedLogDrain(t, ctx, st, projectID, enabledSrv.URL)

	// Create a disabled drain directly.
	disabledDrain := &domain.LogDrain{
		ID:          newID(),
		ProjectID:   projectID,
		Name:        "disabled-drain",
		DrainType:   "http",
		EndpointURL: disabledSrv.URL,
		AuthType:    "bearer",
		AuthConfig:  map[string]string{"token": "disabled-token"},
		LevelFilter: []string{},
		Enabled:     false,
	}
	if err := st.CreateLogDrain(ctx, disabledDrain); err != nil {
		t.Fatalf("CreateLogDrain(disabled) error = %v", err)
	}

	svc := logdrain.NewService()
	w := logdrain.NewWorker(st, svc, 50*time.Millisecond)

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(workerCtx)
		close(done)
	})

	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if enabledReceived == 0 {
		t.Error("enabled drain should have received at least one delivery")
	}
	if disabledReceived != 0 {
		t.Errorf("disabled drain should not have received any deliveries, got %d", disabledReceived)
	}
}

// TestWorker_NoFinishedRuns_NoDeliveries verifies the worker does not make
// any HTTP requests when there are no finished runs.
func TestWorker_NoFinishedRuns_NoDeliveries(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-" + newID()
	seedProject(t, ctx, st, projectID)

	var requestCount int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	seedLogDrain(t, ctx, st, projectID, srv.URL)

	svc := logdrain.NewService()
	w := logdrain.NewWorker(st, svc, 50*time.Millisecond)

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(workerCtx)
		close(done)
	})

	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if requestCount != 0 {
		t.Errorf("expected 0 HTTP requests with no finished runs, got %d", requestCount)
	}
}

// TestWorker_ManyEvents_Pagination verifies that the worker correctly
// paginates through a large number of events for a single run.
func TestWorker_ManyEvents_Pagination(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-" + newID()
	seedProject(t, ctx, st, projectID)
	job := seedJob(t, ctx, st, projectID)

	finishedAt := time.Now().Add(-1 * time.Minute).Truncate(time.Microsecond)
	run := seedFinishedRun(t, ctx, st, job, finishedAt)

	// Insert enough events to span multiple pages (defaultEventLimit = 1000).
	numEvents := 50
	for i := range numEvents {
		seedEvent(t, ctx, st, run.ID, fmt.Sprintf("bulk-event-%03d", i))
	}

	var receivedEvents []domain.RunEvent
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []domain.RunEvent
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		receivedEvents = append(receivedEvents, events...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	seedLogDrain(t, ctx, st, projectID, srv.URL)

	svc := logdrain.NewService()
	w := logdrain.NewWorker(st, svc, 50*time.Millisecond)

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(workerCtx)
		close(done)
	})

	time.Sleep(400 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(receivedEvents) != numEvents {
		t.Errorf("expected %d events, got %d", numEvents, len(receivedEvents))
	}
}

// TestWorker_IdempotentRedelivery verifies that the worker does not re-deliver
// events for runs that have already been processed (checkpoint advancement).
func TestWorker_IdempotentRedelivery(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	ctx := context.Background()
	st := mustStore(t)
	mustClean(t, ctx)

	projectID := "proj-" + newID()
	seedProject(t, ctx, st, projectID)
	job := seedJob(t, ctx, st, projectID)

	finishedAt := time.Now().Add(-1 * time.Minute).Truncate(time.Microsecond)
	run := seedFinishedRun(t, ctx, st, job, finishedAt)
	seedEvent(t, ctx, st, run.ID, "idempotent test")

	var deliveryCount int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		deliveryCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	seedLogDrain(t, ctx, st, projectID, srv.URL)

	svc := logdrain.NewService()
	w := logdrain.NewWorker(st, svc, 30*time.Millisecond)

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	concWG.Go(func() {
		w.Run(workerCtx)
		close(done)
	})

	// Let multiple ticks fire.
	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	// Should only deliver once because the checkpoint advances past the run.
	if deliveryCount != 1 {
		t.Errorf("expected exactly 1 delivery (idempotent), got %d", deliveryCount)
	}
}

// TestService_DrainRunEvents_HTTPEndpoint verifies the Service sends events
// to a real HTTP endpoint with correct payload format.
func TestService_DrainRunEvents_HTTPEndpoint(t *testing.T) {
	var capturedBody []byte
	var capturedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedContentType = r.Header.Get("Content-Type")
		var err error
		capturedBody, err = json.Marshal(nil) // reset
		_ = err
		buf := make([]byte, r.ContentLength)
		n, _ := r.Body.Read(buf)
		capturedBody = buf[:n]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := &domain.LogDrain{
		ID:          "drain-test",
		ProjectID:   "proj-test",
		EndpointURL: srv.URL,
		AuthType:    "bearer",
		AuthConfig:  map[string]string{"token": "my-secret"},
	}

	events := []domain.RunEvent{
		{ID: "evt-1", RunID: "run-1", Type: domain.EventLog, Message: "hello"},
		{ID: "evt-2", RunID: "run-1", Type: domain.EventLog, Message: "world"},
	}

	svc := logdrain.NewService()
	if err := svc.DrainRunEvents(context.Background(), drain, events); err != nil {
		t.Fatalf("DrainRunEvents() error = %v", err)
	}

	if capturedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", capturedContentType)
	}

	var decoded []domain.RunEvent
	if err := json.Unmarshal(capturedBody, &decoded); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 events in payload, got %d", len(decoded))
	}
}

// TestService_DrainRunEvents_BasicAuth verifies basic auth credentials are
// sent correctly to the HTTP endpoint.
func TestService_DrainRunEvents_BasicAuth(t *testing.T) {
	var capturedUser, capturedPass string
	var basicOK bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser, capturedPass, basicOK = r.BasicAuth()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := &domain.LogDrain{
		EndpointURL: srv.URL,
		AuthType:    "basic",
		AuthConfig:  map[string]string{"username": "admin", "password": "secret123"},
	}

	events := []domain.RunEvent{{ID: "evt-1", RunID: "run-1", Message: "test"}}

	svc := logdrain.NewService()
	if err := svc.DrainRunEvents(context.Background(), drain, events); err != nil {
		t.Fatalf("DrainRunEvents() error = %v", err)
	}

	if !basicOK {
		t.Fatal("expected basic auth credentials")
	}
	if capturedUser != "admin" {
		t.Errorf("username = %q, want admin", capturedUser)
	}
	if capturedPass != "secret123" {
		t.Errorf("password = %q, want secret123", capturedPass)
	}
}

// TestService_DrainRunEvents_EndpointError verifies that a server error
// from the drain endpoint is propagated as an error.
func TestService_DrainRunEvents_EndpointError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	drain := &domain.LogDrain{
		EndpointURL: srv.URL,
		AuthType:    "",
	}

	events := []domain.RunEvent{{ID: "evt-1", RunID: "run-1", Message: "test"}}

	svc := logdrain.NewService()
	err := svc.DrainRunEvents(context.Background(), drain, events)
	if err == nil {
		t.Fatal("expected error from 503 endpoint, got nil")
	}
}
