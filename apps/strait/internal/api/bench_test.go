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

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

func benchmarkConfig() *config.Config {
	return &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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
			r.Header.Set("X-Internal-Secret", "test-secret-value")
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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
			r.Header.Set("X-Internal-Secret", "test-secret-value")
			r.Header.Set("Content-Type", "application/json")
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				b.Fatalf("expected 201, got %d", w.Code)
			}
		}
	})
}

func BenchmarkHandleBulkTrigger_500Items(b *testing.B) {
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
	}
	mq := &mockQueue{
		enqueueBatchFn: func(_ context.Context, runs []*domain.JobRun) (int64, error) {
			return int64(len(runs)), nil
		},
	}
	srv := NewServer(ServerDeps{
		Config: benchmarkConfig(),
		Store:  ms,
		Queue:  mq,
	})
	b.Cleanup(srv.Close)

	// Build a 500-item JSON body.
	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	for i := range 500 {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{}`)
	}
	sb.WriteString(`]}`)
	body := sb.String()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", strings.NewReader(body))
			r.Header.Set("X-Internal-Secret", "test-secret-value")
			r.Header.Set("Content-Type", "application/json")
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				b.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
			}
		}
	})
}

func BenchmarkHandleBulkCancel(b *testing.B) {
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, Status: domain.StatusExecuting}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			return nil
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
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
			r.Header.Set("X-Internal-Secret", "test-secret-value")
			r.Header.Set("Content-Type", "application/json")
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				b.Fatalf("expected 200, got %d", w.Code)
			}
		}
	})
}

func BenchmarkHandleStats(b *testing.B) {
	ms := &APIStoreMock{
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
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
			r.Header.Set("X-Internal-Secret", "test-secret-value")
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

	ms := &APIStoreMock{
		ListJobsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.Job, error) {
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
			r.Header.Set("X-Internal-Secret", "test-secret-value")
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
	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, gotHash string) (*domain.APIKey, error) {
			if gotHash != keyHash {
				return nil, fmt.Errorf("unexpected hash")
			}
			return &domain.APIKey{ID: "key-123", ProjectID: "proj-1"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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
	var wg conc.WaitGroup
	errs := make([]error, goroutines)

	for i := range goroutines {
		idx := i
		wg.Go(func() {
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":{}}`)
			r.RemoteAddr = uniqueRemoteAddr(&reqCount)
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				errs[idx] = fmt.Errorf("goroutine %d: expected 201, got %d", idx, w.Code)
			}
		})
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(
		t, int64(goroutines), enqueueCount.
			Load())
}

func TestConcurrentBulkTrigger(t *testing.T) {
	var enqueueCount atomic.Int64
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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

	var wg conc.WaitGroup
	errs := make([]error, goroutines)

	for i := range goroutines {
		idx := i
		wg.Go(func() {
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body)
			r.RemoteAddr = fmt.Sprintf("198.51.100.%d:1234", idx+1)
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusCreated {
				errs[idx] = fmt.Errorf("goroutine %d: expected 201, got %d", idx, w.Code)
			}
		})
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	expected := int64(goroutines * itemsPerRequest)
	assert.Equal(
		t, expected, enqueueCount.
			Load())
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
	ms := &APIStoreMock{
		GetRunsByIDsFunc: func(_ context.Context, ids []string) (map[string]*domain.JobRun, error) {
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
		BulkCancelRunsFunc: func(_ context.Context, ids []string, _ time.Time, _ string) ([]store.BulkCancelResult, error) {
			mu.Lock()
			defer mu.Unlock()
			results := make([]store.BulkCancelResult, 0, len(ids))
			for _, id := range ids {
				cancelAttempts[id]++
				results = append(results, store.BulkCancelResult{ID: id, Canceled: true})
			}
			return results, nil
		},
		CancelChildRunsByParentIDsFunc: func(_ context.Context, _ []string, _ time.Time, _ string) (int64, error) {
			return 0, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	var wg conc.WaitGroup
	errs := make([]error, goroutines)

	for i := range goroutines {
		idx := i
		wg.Go(func() {
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
		})
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}

	mu.Lock()
	defer mu.Unlock()
	for runID := range runs {
		assert.NotEqual(t, 0, cancelAttempts[runID])
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
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
		},
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			mu.Lock()
			defer mu.Unlock()
			st, ok := runs[id]
			if !ok {
				return nil, fmt.Errorf("run not found")
			}
			return &domain.JobRun{ID: id, Status: st}, nil
		},
		UpdateRunStatusFunc: func(_ context.Context, id string, _, to domain.RunStatus, _ map[string]any) error {
			mu.Lock()
			defer mu.Unlock()
			if _, ok := runs[id]; ok {
				runs[id] = to
			}
			return nil
		},
		ListChildRunsFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return nil, nil
		},
		ListRunsByProjectFunc: func(_ context.Context, projectID string, _ *domain.RunStatus, _, _, _, _ *string, _ json.RawMessage, _ *domain.ExecutionMode, _ *string, limit int, _ *time.Time) ([]domain.JobRun, error) {
			out := make([]domain.JobRun, 0, limit)
			for i := range limit {
				out = append(out, domain.JobRun{ID: fmt.Sprintf("list-%d", i), ProjectID: projectID, Status: domain.StatusQueued})
			}
			return out, nil
		},
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
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

	var wg conc.WaitGroup
	errs := make([]error, goroutines)

	for i := range goroutines {
		idx := i
		wg.Go(func() {
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
		})
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(
		t, int64(goroutines/4),
		enqueueCount.
			Load())
}

func TestBurstTraffic(t *testing.T) {
	var enqueueCount atomic.Int64
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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
	for range requests {
		w := httptest.NewRecorder()
		r := authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger", `{"payload":{"burst":true}}`)
		r.RemoteAddr = uniqueRemoteAddr(&reqCount)
		srv.ServeHTTP(w, r)
		require.Equal(t, http.StatusCreated,

			w.Code)
	}
	require.EqualValues(t, requests, enqueueCount.
		Load())
}

func TestSustainedLoad(t *testing.T) {
	var enqueueCount atomic.Int64
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return benchmarkJob(id), nil
		},
		AreJobDependenciesSatisfiedFunc: func(_ context.Context, _ *domain.JobRun) (bool, error) {
			return true, nil
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

	var wg conc.WaitGroup
	errs := make([]error, workers)

	for i := range workers {
		idx := i
		wg.Go(func() {
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
		})
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, totalRequests,
		enqueueCount.
			Load())
}

func TestAPIKeyAuthConcurrent(t *testing.T) {
	rawKey := "strait_" + strings.Repeat("ab", 32)
	wantHash := hashAPIKey(rawKey)
	var touchCount atomic.Int64

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, keyHash string) (*domain.APIKey, error) {
			if keyHash != wantHash {
				return nil, fmt.Errorf("unexpected hash")
			}
			return &domain.APIKey{ID: "key-123", ProjectID: "proj-1"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error {
			touchCount.Add(1)
			return nil
		},
		QueueStatsFunc: func(_ context.Context) (*store.QueueStats, error) {
			return &store.QueueStats{Queued: 1, Executing: 1, Delayed: 1}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	const goroutines = 50
	var wg conc.WaitGroup
	errs := make([]error, goroutines)

	for i := range goroutines {
		idx := i
		wg.Go(func() {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
			r.Header.Set("Authorization", "Bearer "+rawKey)
			srv.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				errs[idx] = fmt.Errorf("goroutine %d: expected 200, got %d", idx, w.Code)
			}
		})
	}
	wg.Wait()

	for _, err := range errs {
		require.NoError(t, err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if touchCount.Load() == goroutines {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.EqualValues(t, goroutines, touchCount.
		Load())
}
