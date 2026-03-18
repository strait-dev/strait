package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

func benchmarkConfig() *config.Config {
	return &config.Config{
		InternalSecret:      "test-secret",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       "01234567890123456789012345678901",
	}
}

func benchmarkJob(id string) *domain.Job {
	return &domain.Job{
		ID:          id,
		ProjectID:   "proj-1",
		Name:        "Bench",
		EndpointURL: "https://example.com",
		Enabled:     true,
		TimeoutSecs: 300,
		MaxAttempts: 3,
	}
}

func uniqueRemoteAddr(counter *atomic.Uint64) string {
	n := counter.Add(1)
	third := (n/250)%250 + 1
	fourth := n%250 + 1
	return fmt.Sprintf("198.51.%d.%d:1234", third, fourth)
}

func BenchmarkHandleTriggerJob(b *testing.B) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
		Queue:  mq,
	})
	b.Cleanup(srv.Close)
	body := `{"payload":{"key":"value"}}`
	var reqCount atomic.Uint64

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-123/trigger", strings.NewReader(body))
			r.Header.Set("X-Internal-Secret", "test-secret")
			r.Header.Set("Content-Type", "application/json")
			r.RemoteAddr = uniqueRemoteAddr(&reqCount)
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				b.Fatalf("expected 201, got %d", w.Code)
			}
		}
	})
}

func BenchmarkHandleBulkTrigger(b *testing.B) {
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil },
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
		Queue:  mq,
	})
	b.Cleanup(srv.Close)
	body := `{"items":[{},{},{},{},{},{},{},{},{},{}]}`

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", strings.NewReader(body))
			r.Header.Set("X-Internal-Secret", "test-secret")
			r.Header.Set("Content-Type", "application/json")
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				b.Fatalf("expected 201, got %d", w.Code)
			}
		}
	})
}

func BenchmarkHandleBulkCancel(b *testing.B) {
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		listChildRunsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, nil
		},
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
		Queue:  &mockQueue{},
	})
	b.Cleanup(srv.Close)
	body := `{"run_ids":["run-1","run-2","run-3","run-4","run-5","run-6","run-7","run-8","run-9","run-10"]}`

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/runs/bulk-cancel", strings.NewReader(body))
			r.Header.Set("X-Internal-Secret", "test-secret")
			r.Header.Set("Content-Type", "application/json")
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				b.Fatalf("expected 200, got %d", w.Code)
			}
		}
	})
}

func BenchmarkHandleStats(b *testing.B) {
	ms := &mockAPIStore{
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 10, Executing: 4, Delayed: 2}, nil
		},
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
		Queue:  &mockQueue{},
	})
	b.Cleanup(srv.Close)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
			r.Header.Set("X-Internal-Secret", "test-secret")
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				b.Fatalf("expected 200, got %d", w.Code)
			}
		}
	})
}

func BenchmarkHandleListJobs(b *testing.B) {
	jobs := make([]domain.Job, 50)
	for i := range jobs {
		jobs[i] = domain.Job{
			ID:          fmt.Sprintf("job-%d", i),
			ProjectID:   "proj-1",
			Name:        fmt.Sprintf("Job %d", i),
			EndpointURL: "https://example.com",
			Enabled:     true,
		}
	}

	ms := &mockAPIStore{
		listJobsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Job, error) {
			return jobs, nil
		},
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
		Queue:  &mockQueue{},
	})
	b.Cleanup(srv.Close)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/v1/jobs/", nil)
			r.Header.Set("X-Internal-Secret", "test-secret")
			r.Header.Set("X-Project-Id", "proj-1")
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				b.Fatalf("expected 200, got %d", w.Code)
			}
		}
	})
}

func BenchmarkAPIKeyAuth(b *testing.B) {
	rawKey := "strait_" + strings.Repeat("ab", 32)
	keyHash := hashAPIKey(rawKey)
	ms := &mockAPIStore{
		getAPIKeyByHashFn: func(_ context.Context, gotHash string) (*domain.APIKey, error) {
			if gotHash != keyHash {
				return nil, fmt.Errorf("unexpected hash")
			}
			return &domain.APIKey{ID: "key-123", ProjectID: "proj-1"}, nil
		},
		touchAPIKeyLastUsedFn: func(_ context.Context, _ string) error { return nil },
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 1, Executing: 1, Delayed: 1}, nil
		},
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
		Queue:  &mockQueue{},
	})
	b.Cleanup(srv.Close)
	auth := "Bearer " + rawKey

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
			r.Header.Set("Authorization", auth)
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				b.Fatalf("expected 200, got %d", w.Code)
			}
		}
	})
}

func TestConcurrentTrigger(t *testing.T) {
	var enqueueCount atomic.Int64
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCount.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	const goroutines = 50
	var reqCount atomic.Uint64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":{}}`)
			r.RemoteAddr = uniqueRemoteAddr(&reqCount)
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				errs[idx] = fmt.Errorf("goroutine %d: expected 201, got %d", idx, w.Code)
			}
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if got := enqueueCount.Load(); got != goroutines {
		t.Errorf("expected %d enqueues, got %d", goroutines, got)
	}
}

func TestConcurrentBulkTrigger(t *testing.T) {
	var enqueueCount atomic.Int64
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCount.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	const goroutines = 20
	const itemsPerRequest = 10
	body := `{"items":[{},{},{},{},{},{},{},{},{},{}]}`

	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body)
			r.RemoteAddr = fmt.Sprintf("198.51.100.%d:1234", idx+1)
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				errs[idx] = fmt.Errorf("goroutine %d: expected 201, got %d", idx, w.Code)
			}
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	expected := int64(goroutines * itemsPerRequest)
	if got := enqueueCount.Load(); got != expected {
		t.Errorf("expected %d enqueues, got %d", expected, got)
	}
}

func TestConcurrentBulkCancel(t *testing.T) {
	const goroutines = 20
	const runsPerRequest = 5
	totalRuns := goroutines * runsPerRequest

	runs := make(map[string]*domain.JobRun, totalRuns)
	for i := range totalRuns {
		id := fmt.Sprintf("run-%d", i)
		runs[id] = &domain.JobRun{ID: id, Status: domain.StatusExecuting}
	}

	var mu sync.Mutex
	cancelAttempts := make(map[string]int, totalRuns)
	ms := &mockAPIStore{
		getRunsByIDsFn: func(_ context.Context, ids []string) (map[string]*domain.JobRun, error) {
			mu.Lock()
			defer mu.Unlock()
			result := make(map[string]*domain.JobRun)
			for _, id := range ids {
				if r, ok := runs[id]; ok {
					result[id] = r
				}
			}
			return result, nil
		},
		bulkCancelRunsFn: func(_ context.Context, ids []string, _ time.Time, _ string) ([]store.BulkCancelResult, error) {
			mu.Lock()
			defer mu.Unlock()
			results := make([]store.BulkCancelResult, 0, len(ids))
			for _, id := range ids {
				cancelAttempts[id]++
				results = append(results, store.BulkCancelResult{ID: id, Canceled: true})
			}
			return results, nil
		},
		cancelChildRunsByParentIDsFn: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			start := idx * runsPerRequest
			runIDs := make([]string, 0, runsPerRequest)
			for j := range runsPerRequest {
				runIDs = append(runIDs, "run-"+strconv.Itoa(start+j))
			}
			payload, err := json.Marshal(map[string]any{"run_ids": runIDs})
			if err != nil {
				errs[idx] = err
				return
			}
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", string(payload))
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				errs[idx] = fmt.Errorf("goroutine %d: expected 200, got %d", idx, w.Code)
			}
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	for runID := range runs {
		if cancelAttempts[runID] == 0 {
			t.Errorf("expected cancel attempt for %s", runID)
		}
	}
}

func TestConcurrentMixedOperations(t *testing.T) {
	const goroutines = 100
	const bulkCancelOps = goroutines / 4
	const runsPerBulkCancel = 5
	var reqCount atomic.Uint64

	var enqueueCount atomic.Int64
	runs := make(map[string]domain.RunStatus, bulkCancelOps*runsPerBulkCancel)
	for i := range bulkCancelOps * runsPerBulkCancel {
		runs[fmt.Sprintf("mix-run-%d", i)] = domain.StatusExecuting
	}

	var mu sync.Mutex
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			mu.Lock()
			defer mu.Unlock()
			st, ok := runs[id]
			if !ok {
				return nil, fmt.Errorf("run not found")
			}
			return &domain.JobRun{ID: id, Status: st}, nil
		},
		updateRunStatusFn: func(_ context.Context, id string, _, to domain.RunStatus, _ map[string]any) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := runs[id]; ok {
				runs[id] = to
			}
			return nil
		},
		listChildRunsFn: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, nil
		},
		listRunsByProjectFn: func(_ context.Context, projectID string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, limit int, _ *time.Time) ([]domain.JobRun, error) {
			out := make([]domain.JobRun, 0, limit)
			for i := range limit {
				out = append(out, domain.JobRun{ID: fmt.Sprintf("list-%d", i), ProjectID: projectID, Status: domain.StatusQueued})
			}
			return out, nil
		},
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 5, Executing: 2, Delayed: 1}, nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCount.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			w := httptest.NewRecorder()

			switch idx % 4 {
			case 0:
				r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":{"n":1}}`)
				r.RemoteAddr = uniqueRemoteAddr(&reqCount)
				srv.ServeHTTP(w, r)
				if w.Code != http.StatusCreated {
					errs[idx] = fmt.Errorf("trigger expected 201, got %d", w.Code)
				}
			case 1:
				start := (idx / 4) * runsPerBulkCancel
				runIDs := make([]string, 0, runsPerBulkCancel)
				for j := range runsPerBulkCancel {
					runIDs = append(runIDs, fmt.Sprintf("mix-run-%d", start+j))
				}
				payload, err := json.Marshal(map[string]any{"run_ids": runIDs})
				if err != nil {
					errs[idx] = err
					return
				}
				r := authedRequest(http.MethodPost, "/v1/runs/bulk-cancel", string(payload))
				srv.ServeHTTP(w, r)
				if w.Code != http.StatusOK {
					errs[idx] = fmt.Errorf("bulk-cancel expected 200, got %d", w.Code)
				}
			case 2:
				r := authedProjectRequest(http.MethodGet, "/v1/runs?limit=10", "", "proj-1")
				srv.ServeHTTP(w, r)
				if w.Code != http.StatusOK {
					errs[idx] = fmt.Errorf("list runs expected 200, got %d", w.Code)
				}
			default:
				r := authedRequest(http.MethodGet, "/v1/stats", "")
				srv.ServeHTTP(w, r)
				if w.Code != http.StatusOK {
					errs[idx] = fmt.Errorf("stats expected 200, got %d", w.Code)
				}
			}
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if got := enqueueCount.Load(); got != goroutines/4 {
		t.Errorf("expected %d trigger enqueues, got %d", goroutines/4, got)
	}
}

func TestBurstTraffic(t *testing.T) {
	var enqueueCount atomic.Int64
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCount.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)
	var reqCount atomic.Uint64

	const requests = 200
	for i := range requests {
		w := httptest.NewRecorder()
		r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":{"burst":true}}`)
		r.RemoteAddr = uniqueRemoteAddr(&reqCount)
		srv.ServeHTTP(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("request %d: expected 201, got %d", i, w.Code)
		}
	}
	if got := enqueueCount.Load(); got != requests {
		t.Fatalf("expected %d enqueues, got %d", requests, got)
	}
}

func TestSustainedLoad(t *testing.T) {
	var enqueueCount atomic.Int64
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
	}
	mq := &mockQueue{
		enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
			enqueueCount.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, mq, nil)
	var reqCount atomic.Uint64

	const workers = 10
	const requestsPerWorker = 30
	const totalRequests = workers * requestsPerWorker

	var wg sync.WaitGroup
	wg.Add(workers)
	errs := make([]error, workers)

	for i := range workers {
		go func(idx int) {
			defer wg.Done()
			for j := range requestsPerWorker {
				w := httptest.NewRecorder()
				r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":{"sustained":true}}`)
				r.RemoteAddr = uniqueRemoteAddr(&reqCount)
				srv.ServeHTTP(w, r)
				if w.Code != http.StatusCreated {
					errs[idx] = fmt.Errorf("worker %d request %d: expected 201, got %d", idx, j, w.Code)
					return
				}
				time.Sleep(2 * time.Millisecond)
			}
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if got := enqueueCount.Load(); got != totalRequests {
		t.Fatalf("expected %d enqueues, got %d", totalRequests, got)
	}
}

func TestAPIKeyAuthConcurrent(t *testing.T) {
	rawKey := "strait_" + strings.Repeat("ab", 32)
	wantHash := hashAPIKey(rawKey)
	var touchCount atomic.Int64

	ms := &mockAPIStore{
		getAPIKeyByHashFn: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			if keyHash != wantHash {
				return nil, fmt.Errorf("unexpected hash")
			}
			return &domain.APIKey{ID: "key-123", ProjectID: "proj-1"}, nil
		},
		touchAPIKeyLastUsedFn: func(_ context.Context, _ string) error {
			touchCount.Add(1)
			return nil
		},
		queueStatsFn: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 1, Executing: 1, Delayed: 1}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
			r.Header.Set("Authorization", "Bearer "+rawKey)
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				errs[idx] = fmt.Errorf("goroutine %d: expected 200, got %d", idx, w.Code)
			}
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if touchCount.Load() == goroutines {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := touchCount.Load(); got != goroutines {
		t.Fatalf("expected %d touch calls, got %d", goroutines, got)
	}
}
