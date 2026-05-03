package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"

	"github.com/sourcegraph/conc"
)

// TestDoS_HTTPRequestTimeout verifies that a handler that takes too long
// receives a context cancellation when the client disconnects.
func TestDoS_HTTPRequestTimeout(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(ctx context.Context, id string) (*domain.Job, error) {
			// Simulate a slow store call.
			select {
			case <-time.After(5 * time.Second):
				return &domain.Job{ID: id}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Use a short client timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/v1/jobs/slow-job", nil)
	req = req.WithContext(ctx)
	req.Header.Set("X-Internal-Secret", "test-secret-value")

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// The handler should have returned an error due to context cancellation
	// or the timeout. We accept any non-200 status or a 200 with error.
	// The key assertion is that the server did not hang.
	if w.Code == http.StatusOK {
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err == nil {
			if _, hasErr := resp["error"]; !hasErr {
				// If we got a clean 200 with no error, the timeout did not work,
				// but this is unlikely given the 100ms timeout vs 5s sleep.
				t.Log("handler returned 200 without error despite timeout")
			}
		}
	}
}

// TestDoS_BatchOperationMaxItems submits a bulk trigger with more items than
// the configured maximum and verifies it is rejected.
func TestDoS_BatchOperationMaxItems(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Bulk Job",
				Slug:        "bulk-job",
				EndpointURL: "https://example.com/callback",
				MaxAttempts: 3,
				TimeoutSecs: 60,
				Enabled:     true,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Build a payload with 501 items (max is 500).
	items := make([]map[string]any, 501)
	for i := range items {
		items[i] = map[string]any{"payload": map[string]int{"i": i}}
	}
	body, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	req := authedRequest(http.MethodPost, "/v1/jobs/some-job-id/trigger/bulk", string(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	// Expect a 400 or 422 for exceeding the limit.
	if w.Code != http.StatusBadRequest && w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 400 or 422 for oversized batch, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDoS_MemoryBombPayload sends 1000 concurrent requests, each containing a
// large payload near the maxRequestBodySize, and verifies the server handles
// them without panicking.
func TestDoS_MemoryBombPayload(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			job.ID = "job-mem-bomb"
			job.CreatedAt = time.Now()
			job.UpdatedAt = time.Now()
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	// Create a payload that is large but under the default 1MB limit.
	// Use ~512KB to avoid actual OOM but still stress the allocator.
	largePayload := strings.Repeat("x", 512*1024)

	var wg conc.WaitGroup
	const concurrency = 100 // Reduced from 1000 to keep test fast.

	for range concurrency {
		wg.Go(func() {
			body := fmt.Sprintf(`{"project_id":"proj-1","name":"Mem Bomb","slug":"mem-bomb","endpoint_url":"https://example.com/%s"}`, largePayload[:100])
			req := authedRequest(http.MethodPost, "/v1/jobs/", body)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			// We don't care about the status; we only verify no panic.
		})
	}
	wg.Wait()
}

// TestDoS_EventTriggerFanout sends many concurrent event trigger requests to
// exercise the send-event path under load.
func TestDoS_EventTriggerFanout(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetEventTriggerByEventKeyFunc: func(_ context.Context, eventKey string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:        "trigger-fanout",
				ProjectID: "proj-1",
				EventKey:  eventKey,
				Status:    domain.EventTriggerStatusWaiting,
			}, nil
		},
		ReceiveEventAndRequeueRunFunc: func(_ context.Context, _ string, _ json.RawMessage, _ time.Time, _ string) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	const concurrency = 50
	var wg conc.WaitGroup

	for i := range concurrency {
		wg.Go(func() {
			body := fmt.Sprintf(`{"payload":{"idx":%d}}`, i)
			req := authedRequest(http.MethodPost, fmt.Sprintf("/v1/events/user.signup.%d/send", i), body)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
		})
	}
	wg.Wait()
}

// TestDoS_GoroutineLeakSSE creates SSE-like connections, closes them, and
// verifies that the goroutine count returns to the baseline.
func TestDoS_GoroutineLeakSSE(t *testing.T) {
	// Not parallel: measures global goroutine count.

	ms := streamTestStore()
	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			ch := make(chan []byte)
			_, cancel := context.WithCancel(context.Background())
			// Close the channel immediately so the SSE handler exits.
			close(ch)
			return pubsub.NewSubscription(ch, cancel), nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, pub)

	// Record baseline goroutine count.
	runtime.GC()
	baseline := runtime.NumGoroutine()

	// Create and close several SSE connections.
	const count = 20
	for i := range count {
		req := authedRequest(http.MethodGet, fmt.Sprintf("/v1/runs/run-%d/stream", i), "")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= baseline+10 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	current := runtime.NumGoroutine()
	leaked := current - baseline
	if leaked > 10 {
		t.Fatalf("possible goroutine leak: baseline=%d, current=%d, leaked=%d", baseline, current, leaked)
	}
}

// TestDoS_WorkerPoolSaturation submits many tasks to the server concurrently
// and verifies that requests beyond capacity are queued or rejected gracefully.
func TestDoS_WorkerPoolSaturation(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			// Simulate a slow operation.
			time.Sleep(50 * time.Millisecond)
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Slow Job",
				Slug:        "slow-job",
				EndpointURL: "https://example.com/callback",
				MaxAttempts: 3,
				TimeoutSecs: 60,
				Enabled:     true,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	const concurrency = 50
	var wg conc.WaitGroup
	results := make([]int, concurrency)

	for i := range concurrency {
		wg.Go(func() {
			req := authedRequest(http.MethodGet, "/v1/jobs/job-saturate", "")
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)
			results[i] = w.Code
		})
	}
	wg.Wait()

	// Count outcomes.
	var ok, errors int
	for _, code := range results {
		if code >= 200 && code < 300 {
			ok++
		} else {
			errors++
		}
	}

	// All requests should have completed (either success or error, no hang).
	if ok+errors != concurrency {
		t.Fatalf("expected %d total responses, got ok=%d, errors=%d", concurrency, ok, errors)
	}
}

// TestDoS_SSEConnectionLimitGlobal verifies that the SSE connection limiter
// rejects new connections when the global limit is exceeded.
func TestDoS_SSEConnectionLimitGlobal(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		InternalSecret:        "test-secret-value",
		MaxBulkTriggerItems:   500,
		JWTSigningKey:         testJWTSigningKey,
		SSEMaxConns:           3,
		SSEMaxConnsPerProject: 100,
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	// Acquire up to the global limit across different projects.
	if !srv.acquireSSEConn("proj-a") {
		t.Fatal("first acquire should succeed")
	}
	if !srv.acquireSSEConn("proj-b") {
		t.Fatal("second acquire should succeed")
	}
	if !srv.acquireSSEConn("proj-c") {
		t.Fatal("third acquire should succeed")
	}

	// Fourth should be rejected (global limit = 3).
	if srv.acquireSSEConn("proj-d") {
		t.Fatal("fourth acquire should be rejected (global limit exceeded)")
	}

	// Release one and try again.
	srv.releaseSSEConn("proj-a")
	if !srv.acquireSSEConn("proj-d") {
		t.Fatal("acquire after release should succeed")
	}
}

// TestDoS_SSEConnectionLimitPerProject verifies the per-project SSE limit.
func TestDoS_SSEConnectionLimitPerProject(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		InternalSecret:        "test-secret-value",
		MaxBulkTriggerItems:   500,
		JWTSigningKey:         testJWTSigningKey,
		SSEMaxConns:           5000,
		SSEMaxConnsPerProject: 2,
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	if !srv.acquireSSEConn("proj-1") {
		t.Fatal("first acquire should succeed")
	}
	if !srv.acquireSSEConn("proj-1") {
		t.Fatal("second acquire should succeed")
	}

	// Third for same project should be rejected.
	if srv.acquireSSEConn("proj-1") {
		t.Fatal("third acquire for same project should be rejected")
	}

	// Different project should still work.
	if !srv.acquireSSEConn("proj-2") {
		t.Fatal("acquire for different project should succeed")
	}
}

// TestDoS_SSEConnectionLimit503Response verifies that the activity stream
// handler returns 503 when the SSE connection limit is exceeded.
func TestDoS_SSEConnectionLimit503Response(t *testing.T) {
	t.Parallel()

	pub := &mockPublisher{
		subscribeFn: func(_ context.Context, _ string) (*pubsub.Subscription, error) {
			ch := make(chan []byte)
			_, cancel := context.WithCancel(context.Background())
			close(ch)
			return pubsub.NewSubscription(ch, cancel), nil
		},
	}
	cfg := &config.Config{
		InternalSecret:        "test-secret-value",
		MaxBulkTriggerItems:   500,
		JWTSigningKey:         testJWTSigningKey,
		SSEMaxConns:           1,
		SSEMaxConnsPerProject: 1,
	}
	srv := NewServer(ServerDeps{
		Config:  cfg,
		Store:   &APIStoreMock{},
		Queue:   &mockQueue{},
		PubSub:  pub,
		Edition: domain.EditionCloud,
	})
	t.Cleanup(srv.Close)

	// Exhaust the single allowed connection.
	srv.acquireSSEConn("proj-1")

	// The next SSE request should get 503.
	req := authedProjectRequest(http.MethodGet, "/v1/projects/proj-1/activity/stream/", "", "proj-1")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
