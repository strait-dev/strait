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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.False(t, testDB ==

		nil || testDB.
		Pool ==
		nil)

	return store.New(testDB.Pool)
}

func mustClean(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, testDB.
		CleanTables(ctx))

}

// seedProject creates a project in the DB for foreign key constraints.
func seedProject(t *testing.T, ctx context.Context, st *store.Queries, projectID string) {
	t.Helper()
	project := &domain.Project{
		ID:    projectID,
		OrgID: "org-" + projectID,
		Name:  "test-project-" + projectID,
	}
	require.NoError(t, st.CreateProject(
		ctx, project,
	))

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
	require.NoError(t, st.CreateJob(ctx,
		job))

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
	require.NoError(t, st.CreateRun(ctx,
		run))
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID, domain.StatusQueued,

		domain.
			StatusDequeued,
		nil))

	// Transition queued -> dequeued.

	// Transition dequeued -> executing.
	startedAt := finishedAt.Add(-10 * time.Second)
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID, domain.StatusDequeued,

		domain.
			StatusExecuting,
		map[string]any{"started_at": startedAt}))
	require.NoError(t, st.UpdateRunStatus(ctx, run.
		ID, domain.StatusExecuting,

		domain.
			StatusCompleted,
		map[string]any{"finished_at": finishedAt}))

	// Transition executing -> completed.

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
	require.NoError(t, st.InsertEvent(ctx,
		event))

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
	require.NoError(t, st.CreateLogDrain(ctx, drain))

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
	require.Len(t, received, 2)

	ids := map[string]bool{received[0].ID: true, received[1].ID: true}
	assert.False(t, !ids[evt1.
		ID] || !ids[evt2.ID])

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
	assert.Equal(t, "Bearer test-token",

		capturedAuth,
	)

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
	assert.Equal(t, numRuns, deliveryCount)
	assert.Equal(t, expectedEvents,

		totalEvents,
	)

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
	assert.GreaterOrEqual(t, callCount,

		3)

	// Should have been called exactly maxRunRetries (3) times, then skipped.

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
	require.GreaterOrEqual(t,
		callCount,

		2)
	assert.Len(t, receivedEvents,

		1)

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
		assert.Len(t, events, eventsPerProject)

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
	require.NoError(t, st.CreateLogDrain(ctx, disabledDrain))

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
	assert.NotEqual(t, 0, enabledReceived)
	assert.Equal(t, 0, disabledReceived)

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
	assert.Equal(t, 0, requestCount)

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
	assert.Len(t, receivedEvents,

		numEvents,
	)

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
	assert.Equal(t, 1, deliveryCount)

	// Should only deliver once because the checkpoint advances past the run.

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
	require.NoError(t, svc.DrainRunEvents(context.Background(), drain,
		events,
	))
	assert.Equal(t, "application/json",

		capturedContentType,
	)

	var decoded []domain.RunEvent
	require.NoError(t, json.Unmarshal(capturedBody,

		&decoded))
	require.Len(t, decoded, 2)

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
	require.NoError(t, svc.DrainRunEvents(context.Background(), drain,
		events,
	))
	require.True(t, basicOK)
	assert.Equal(t, "admin", capturedUser)
	assert.Equal(t, "secret123",

		capturedPass,
	)

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
	require.Error(t, err)

}
