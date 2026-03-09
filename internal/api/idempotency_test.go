package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// Basic idempotency key handling.

func TestIdempotency_XHeader_ReturnsExistingRun(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			if key != "my-key-1" {
				t.Fatalf("unexpected key: %s", key)
			}
			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "my-key-1")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued {
		t.Fatal("expected enqueue to be skipped for idempotency hit")
	}

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	if resp["id"] != "run-existing" {
		t.Fatalf("expected existing run id, got %v", resp["id"])
	}
}

func TestIdempotency_StandardHeader_ReturnsExistingRun(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, key string) (*domain.JobRun, error) {
			lookupCalled = true
			if key != "standard-key" {
				t.Fatalf("expected key 'standard-key', got %q", key)
			}
			return &domain.JobRun{ID: "run-std", Status: domain.StatusExecuting}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("Idempotency-Key", "standard-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !lookupCalled {
		t.Fatal("expected idempotency lookup to be called via standard header")
	}

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	if resp["id"] != "run-std" {
		t.Fatalf("expected run-std, got %v", resp["id"])
	}
}

func TestIdempotency_XHeaderTakesPrecedenceOverStandard(t *testing.T) {
	t.Parallel()
	var capturedKey string
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, key string) (*domain.JobRun, error) {
			capturedKey = key
			return &domain.JobRun{ID: "run-x", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "x-key")
	r.Header.Set("Idempotency-Key", "standard-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if capturedKey != "x-key" {
		t.Fatalf("expected X-Idempotency-Key to take precedence, got key=%q", capturedKey)
	}
}

func TestIdempotency_NoHeaderSkipsLookup(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	// No idempotency header
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if lookupCalled {
		t.Fatal("expected idempotency lookup to be skipped when no header present")
	}
}

func TestIdempotency_EmptyHeaderSkipsLookup(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if lookupCalled {
		t.Fatal("expected idempotency lookup to be skipped for empty header value")
	}
}

func TestIdempotency_MissCreatesNewRun(t *testing.T) {
	t.Parallel()
	var enqueued *domain.JobRun
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil // not found
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		enqueued = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"data":"test"}}`)
	r.Header.Set("X-Idempotency-Key", "new-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued == nil {
		t.Fatal("expected run to be enqueued on idempotency miss")
	}
	if enqueued.IdempotencyKey != "new-key" {
		t.Fatalf("expected idempotency key 'new-key' on enqueued run, got %q", enqueued.IdempotencyKey)
	}
}

// Response shape verification.

func TestIdempotency_HitResponseShape(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-cached", Status: domain.StatusExecuting}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "cached-key")
	srv.ServeHTTP(w, r)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)

	// Hit response should have id and status
	if resp["id"] != "run-cached" {
		t.Fatalf("expected id=run-cached, got %v", resp["id"])
	}
	if resp["status"] != "executing" {
		t.Fatalf("expected status=executing, got %v", resp["status"])
	}

	// Hit response should NOT have run_token or payload_hash
	if _, ok := resp["run_token"]; ok {
		t.Fatal("idempotency hit response should NOT contain run_token")
	}
	if _, ok := resp["payload_hash"]; ok {
		t.Fatal("idempotency hit response should NOT contain payload_hash")
	}
}

func TestIdempotency_MissResponseShape(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "new-key")
	srv.ServeHTTP(w, r)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)

	// Miss response should have id, status, payload_hash, and run_token
	if resp["id"] == nil || resp["id"] == "" {
		t.Fatal("miss response should contain id")
	}
	if resp["status"] == nil {
		t.Fatal("miss response should contain status")
	}
	if resp["run_token"] == nil || resp["run_token"] == "" {
		t.Fatal("miss response should contain run_token")
	}
	if resp["payload_hash"] == nil || resp["payload_hash"] == "" {
		t.Fatal("miss response should contain payload_hash")
	}
}

func TestIdempotency_HitReturnsCurrentStatus(t *testing.T) {
	t.Parallel()
	statuses := []domain.RunStatus{
		domain.StatusQueued,
		domain.StatusDequeued,
		domain.StatusExecuting,
		domain.StatusWaiting,
		domain.StatusCompleted,
		domain.StatusFailed,
		domain.StatusTimedOut,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			ms := &mockAPIStore{
				getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
					return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
				},
				getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
					return &domain.JobRun{ID: "run-1", Status: status}, nil
				},
			}

			srv := newTestServer(t, ms, &mockQueue{}, nil)
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", "key-"+string(status))
			srv.ServeHTTP(w, r)

			if w.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
			}

			var resp map[string]any
			mustUnmarshal(t, w.Body.Bytes(), &resp)
			if resp["status"] != string(status) {
				t.Fatalf("expected status=%s, got %v", status, resp["status"])
			}
		})
	}
}

// Idempotency scoped per job.

func TestIdempotency_ScopedPerJob(t *testing.T) {
	t.Parallel()
	lookupArgs := make(map[string]string)
	var mu sync.Mutex

	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			mu.Lock()
			lookupArgs[jobID] = key
			mu.Unlock()
			if jobID == "job-A" {
				return &domain.JobRun{ID: "run-A", Status: domain.StatusQueued}, nil
			}
			return nil, nil // miss for job-B
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)

	// Same key, job-A -> hit
	w1 := httptest.NewRecorder()
	r1 := authedRequest(http.MethodPost, "/v1/jobs/job-A/trigger", `{}`)
	r1.Header.Set("X-Idempotency-Key", "shared-key")
	srv.ServeHTTP(w1, r1)

	// Same key, job-B -> miss
	w2 := httptest.NewRecorder()
	r2 := authedRequest(http.MethodPost, "/v1/jobs/job-B/trigger", `{}`)
	r2.Header.Set("X-Idempotency-Key", "shared-key")
	srv.ServeHTTP(w2, r2)

	if w1.Code != http.StatusCreated || w2.Code != http.StatusCreated {
		t.Fatalf("expected both 201, got %d and %d", w1.Code, w2.Code)
	}

	var resp1, resp2 map[string]any
	mustUnmarshal(t, w1.Body.Bytes(), &resp1)
	mustUnmarshal(t, w2.Body.Bytes(), &resp2)

	if resp1["id"] != "run-A" {
		t.Fatalf("expected job-A hit to return run-A, got %v", resp1["id"])
	}
	// job-B should create a new run (different ID)
	if resp2["id"] == "run-A" {
		t.Fatal("expected job-B to create a different run, but got same as job-A")
	}
	if _, ok := resp2["run_token"]; !ok {
		t.Fatal("expected job-B miss to include run_token")
	}
}

// Error handling.

func TestIdempotency_StoreLookupError_Returns500(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, errors.New("connection refused")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "err-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "idempotency key") {
		t.Fatalf("expected error message about idempotency key, got %s", w.Body.String())
	}
}

func TestIdempotency_StoreLookupError_DoesNotEnqueue(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, errors.New("timeout")
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "err-key")
	srv.ServeHTTP(w, r)

	if enqueued {
		t.Fatal("should NOT enqueue when idempotency lookup fails")
	}
}

// Idempotency key stored on the enqueued run.

func TestIdempotency_KeyStoredOnEnqueuedRun(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRun = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "persist-key-123")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if capturedRun == nil {
		t.Fatal("expected run to be captured")
	}
	if capturedRun.IdempotencyKey != "persist-key-123" {
		t.Fatalf("expected idempotency key on run, got %q", capturedRun.IdempotencyKey)
	}
}

func TestIdempotency_NoKeyStoresEmptyOnRun(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRun = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if capturedRun == nil {
		t.Fatal("expected run to be captured")
	}
	if capturedRun.IdempotencyKey != "" {
		t.Fatalf("expected empty idempotency key when no header, got %q", capturedRun.IdempotencyKey)
	}
}

// Interaction with other features.

func TestIdempotency_HitBeforeRateLimitCheck(t *testing.T) {
	// This test verifies the current behavior: rate limit is checked BEFORE
	// idempotency. If rate limit is exceeded but an idempotent run exists,
	// the request will be rejected with 429.
	// This documents the current ordering for regression purposes.
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60,
				RateLimitMax: 1, RateLimitWindowSecs: 60,
			}, nil
		},
		countRunsForJobSinceFn: func(_ context.Context, _ string, _ time.Time) (int, error) {
			return 1, nil // at limit
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			// This should not be reached because rate limit check happens first
			return &domain.JobRun{ID: "run-cached", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "rate-limit-key")
	srv.ServeHTTP(w, r)

	// Current behavior: rate limit takes precedence
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 (rate limit before idempotency), got %d: %s", w.Code, w.Body.String())
	}
}

func TestIdempotency_HitBeforeDedupCheck(t *testing.T) {
	// Idempotency is checked BEFORE dedup. If idempotency hits, dedup is skipped.
	t.Parallel()
	dedupCalled := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60,
				DedupWindowSecs: 300,
			}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-idem", Status: domain.StatusQueued}, nil
		},
		findRecentRunByPayloadFn: func(_ context.Context, _ string, _ json.RawMessage, _ time.Time) (*domain.JobRun, error) {
			dedupCalled = true
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"x":1}}`)
	r.Header.Set("X-Idempotency-Key", "idem-before-dedup")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if dedupCalled {
		t.Fatal("expected dedup to be skipped when idempotency hits")
	}
}

func TestIdempotency_HitWithProjectQuotaExceeded(t *testing.T) {
	// When project quota is exceeded but idempotency check happens after quota,
	// the request should be rejected.
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getProjectQuotaFn: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{ProjectID: "proj-1", MaxQueuedRuns: 1}, nil
		},
		countProjectQueuedRunsFn: func(_ context.Context, _ string) (int, error) {
			return 1, nil // at limit
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-cached", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFProjectQuotas = true

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "quota-key")
	srv.ServeHTTP(w, r)

	// Current behavior: quota check happens before idempotency
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 (quota check before idempotency), got %d: %s", w.Code, w.Body.String())
	}
}

func TestIdempotency_WithScheduledRun(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			scheduledAt := time.Now().Add(1 * time.Hour)
			return &domain.JobRun{
				ID:          "run-delayed",
				Status:      domain.StatusDelayed,
				ScheduledAt: &scheduledAt,
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "delayed-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	if resp["id"] != "run-delayed" {
		t.Fatalf("expected run-delayed, got %v", resp["id"])
	}
	if resp["status"] != "delayed" {
		t.Fatalf("expected delayed status, got %v", resp["status"])
	}
}

func TestIdempotency_WithDifferentPayloads(t *testing.T) {
	// Same idempotency key with different payload should still return cached run
	// (idempotency key takes priority over payload content)
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:      "run-original",
				Status:  domain.StatusQueued,
				Payload: json.RawMessage(`{"x":1}`),
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"completely":"different"}}`)
	r.Header.Set("X-Idempotency-Key", "same-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	if resp["id"] != "run-original" {
		t.Fatalf("expected original run even with different payload, got %v", resp["id"])
	}
}

// Edge cases.

func TestIdempotency_VeryLongKey(t *testing.T) {
	t.Parallel()
	longKey := strings.Repeat("a", 1024)
	var capturedKey string

	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, key string) (*domain.JobRun, error) {
			capturedKey = key
			return nil, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", longKey)
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if capturedKey != longKey {
		t.Fatalf("expected long key to be passed through, len=%d vs %d", len(capturedKey), len(longKey))
	}
}

func TestIdempotency_SpecialCharactersInKey(t *testing.T) {
	t.Parallel()
	specialKeys := []string{
		"key with spaces",
		"key/with/slashes",
		"key:with:colons",
		"key=with=equals",
		"key+with+plus",
		"key@with@at",
		"key#with#hash",
		"key?with?question",
		"key&with&ampersand",
		"key中文字符",
		"key🚀emoji",
		"key\twith\ttabs",
	}

	for _, key := range specialKeys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			var capturedKey string
			ms := &mockAPIStore{
				getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
					return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
				},
				getRunByIdempotencyKeyFn: func(_ context.Context, _, k string) (*domain.JobRun, error) {
					capturedKey = k
					return nil, nil
				},
			}
			mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

			srv := newTestServer(t, ms, mq, nil)
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", key)
			srv.ServeHTTP(w, r)

			if w.Code != http.StatusCreated {
				t.Fatalf("expected 201 for key %q, got %d: %s", key, w.Code, w.Body.String())
			}
			if capturedKey != key {
				t.Fatalf("expected key %q, got %q", key, capturedKey)
			}
		})
	}
}

func TestIdempotency_WhitespaceOnlyKey(t *testing.T) {
	// A whitespace-only key is technically non-empty, should trigger lookup
	t.Parallel()
	lookupCalled := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "   ")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	if !lookupCalled {
		t.Fatal("expected whitespace-only key to trigger idempotency lookup")
	}
}

func TestIdempotency_DisabledJobStillRejectsBeforeIdempotencyCheck(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: false, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return &domain.JobRun{ID: "run-cached", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "disabled-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for disabled job, got %d", w.Code)
	}
	if lookupCalled {
		t.Fatal("idempotency lookup should not be called for disabled job")
	}
}

func TestIdempotency_JobNotFoundRejectsBeforeIdempotencyCheck(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/nonexistent/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "not-found-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if lookupCalled {
		t.Fatal("idempotency lookup should not be called when job not found")
	}
}

func TestIdempotency_InvalidBodyRejectsBeforeIdempotencyCheck(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{invalid json`)
	r.Header.Set("X-Idempotency-Key", "bad-body-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if lookupCalled {
		t.Fatal("idempotency lookup should not be called with invalid body")
	}
}

// Concurrent requests with same idempotency key.

func TestIdempotency_ConcurrentRequestsSameKey(t *testing.T) {
	t.Parallel()
	var enqueueCount atomic.Int32
	var lookupCount atomic.Int32

	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCount.Add(1)
			return nil, nil // all miss (simulating race)
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueueCount.Add(1)
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	const concurrency = 10
	var wg sync.WaitGroup
	results := make([]*httptest.ResponseRecorder, concurrency)

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", "concurrent-key")
			srv.ServeHTTP(w, r)
			results[idx] = w
		}(i)
	}
	wg.Wait()

	// All should succeed (201) at the application level
	for i, w := range results {
		if w.Code != http.StatusCreated {
			t.Errorf("request %d: expected 201, got %d: %s", i, w.Code, w.Body.String())
		}
	}

	// All should have been looked up
	if lookupCount.Load() != int32(concurrency) {
		t.Errorf("expected %d lookups, got %d", concurrency, lookupCount.Load())
	}

	// NOTE: Without DB-level uniqueness, all would enqueue. The DB unique
	// partial index prevents duplicates at the database level. This test
	// verifies the application-level behavior is correct.
	t.Logf("concurrent enqueue count: %d (DB unique index would prevent duplicates)", enqueueCount.Load())
}

func TestIdempotency_ConcurrentRequestsMixedHitMiss(t *testing.T) {
	t.Parallel()
	var lookupCount atomic.Int32

	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			count := lookupCount.Add(1)
			// First request misses, subsequent ones hit
			if count == 1 {
				return nil, nil
			}
			return &domain.JobRun{ID: "run-first", Status: domain.StatusQueued}, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)

	const concurrency = 5
	var wg sync.WaitGroup
	responses := make([]map[string]any, concurrency)

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", "mixed-key")
			srv.ServeHTTP(w, r)
			var resp map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			responses[idx] = resp
		}(i)
	}
	wg.Wait()

	// Verify all got 201 responses
	for i, resp := range responses {
		if resp == nil {
			t.Errorf("request %d: nil response", i)
			continue
		}
		if resp["id"] == nil || resp["id"] == "" {
			t.Errorf("request %d: missing id in response", i)
		}
	}
}

// Idempotency + replay flow.

func TestIdempotency_ReplayPreservesIdempotencyKey(t *testing.T) {
	t.Parallel()
	var enqueued *domain.JobRun
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:             "run-original",
				JobID:          "job-1",
				ProjectID:      "proj-1",
				Status:         domain.StatusFailed,
				Payload:        json.RawMessage(`{"x":1}`),
				IdempotencyKey: "my-idem-key",
				JobVersion:     1,
			}, nil
		},
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		enqueued = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	srv.config.FFRunReplay = true

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-original/replay", ""))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if enqueued == nil {
		t.Fatal("expected run to be enqueued")
	}
	if enqueued.IdempotencyKey != "my-idem-key" {
		t.Fatalf("expected idempotency key to be preserved on replay, got %q", enqueued.IdempotencyKey)
	}
}

// Idempotency + payload validation.

func TestIdempotency_PayloadValidationFailsBeforeIdempotencyCheck(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:            id,
				ProjectID:     "proj-1",
				Enabled:       true,
				TimeoutSecs:   60,
				PayloadSchema: json.RawMessage(`{"type":"object","required":["name"]}`),
			}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFPayloadValidation = true

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"age":12}}`)
	r.Header.Set("X-Idempotency-Key", "schema-fail-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if lookupCalled {
		t.Fatal("idempotency lookup should not be called when payload validation fails")
	}
}

// Multiple sequential triggers with same key.

func TestIdempotency_SequentialTriggersReturnSameRun(t *testing.T) {
	t.Parallel()
	var enqueueCount int
	existingRun := &domain.JobRun{ID: "run-first", Status: domain.StatusQueued}
	callNum := 0

	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			callNum++
			if callNum == 1 {
				return nil, nil // first call: miss
			}
			return existingRun, nil // subsequent calls: hit
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueueCount++
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	// First trigger: should enqueue
	w1 := httptest.NewRecorder()
	r1 := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r1.Header.Set("X-Idempotency-Key", "seq-key")
	srv.ServeHTTP(w1, r1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("first trigger: expected 201, got %d", w1.Code)
	}
	if enqueueCount != 1 {
		t.Fatalf("expected 1 enqueue after first trigger, got %d", enqueueCount)
	}

	// Second trigger: should return cached
	w2 := httptest.NewRecorder()
	r2 := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r2.Header.Set("X-Idempotency-Key", "seq-key")
	srv.ServeHTTP(w2, r2)

	if w2.Code != http.StatusCreated {
		t.Fatalf("second trigger: expected 201, got %d", w2.Code)
	}
	if enqueueCount != 1 {
		t.Fatalf("expected still 1 enqueue after second trigger, got %d", enqueueCount)
	}

	var resp2 map[string]any
	mustUnmarshal(t, w2.Body.Bytes(), &resp2)
	if resp2["id"] != "run-first" {
		t.Fatalf("expected cached run ID, got %v", resp2["id"])
	}

	// Third trigger: should also return cached
	w3 := httptest.NewRecorder()
	r3 := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"different":"data"}}`)
	r3.Header.Set("X-Idempotency-Key", "seq-key")
	srv.ServeHTTP(w3, r3)

	if w3.Code != http.StatusCreated {
		t.Fatalf("third trigger: expected 201, got %d", w3.Code)
	}
	if enqueueCount != 1 {
		t.Fatalf("expected still 1 enqueue after third trigger, got %d", enqueueCount)
	}
}

// Idempotency + execution window.

func TestIdempotency_ExecutionWindowDoesNotApplyOnHit(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:                  id,
				ProjectID:           "proj-1",
				Enabled:             true,
				TimeoutSecs:         60,
				ExecutionWindowCron: "0 0 1 1 *", // very restrictive window
				Timezone:            "UTC",
			}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-windowed", Status: domain.StatusDelayed}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFExecutionWindows = true

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "window-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	if resp["id"] != "run-windowed" {
		t.Fatalf("expected cached run, got %v", resp["id"])
	}
}

// Idempotency + dry run.

func TestIdempotency_DryRunDoesNotCheckIdempotency(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFDryRun = true

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"dry_run": true}`)
	r.Header.Set("X-Idempotency-Key", "dry-run-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if lookupCalled {
		t.Fatal("dry run should not check idempotency key")
	}
}

// Cost budget + idempotency ordering.

func TestIdempotency_CostBudgetExceeded_RejectsBeforeIdempotency(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getProjectQuotaFn: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{
				ProjectID:            "proj-1",
				MaxDailyCostMicrousd: 100,
			}, nil
		},
		sumProjectDailyCostMicrousdFn: func(_ context.Context, _ string, _ string) (int64, error) {
			return 100, nil // at budget
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-cached", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	srv.config.FFCostBudgets = true

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "budget-key")
	srv.ServeHTTP(w, r)

	// Cost budget check happens before idempotency
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for cost budget exceeded, got %d: %s", w.Code, w.Body.String())
	}
}

// Idempotency key with different HTTP status scenarios.

func TestIdempotency_EnqueueFailureAfterMiss(t *testing.T) {
	t.Parallel()
	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil // miss
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		return fmt.Errorf("queue full")
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "enqueue-fail-key")
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on enqueue failure, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIdempotency_MultipleJobsSameKeyIndependent(t *testing.T) {
	// Verify that the same idempotency key used for multiple different jobs
	// creates independent runs for each job.
	t.Parallel()
	runs := make(map[string]*domain.JobRun)
	var mu sync.Mutex

	ms := &mockAPIStore{
		getJobFn: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		getRunByIdempotencyKeyFn: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			mu.Lock()
			defer mu.Unlock()
			if r, ok := runs[jobID]; ok {
				return r, nil
			}
			return nil, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		mu.Lock()
		runs[run.JobID] = run
		mu.Unlock()
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	jobIDs := []string{"job-A", "job-B", "job-C"}
	responseIDs := make(map[string]string)

	for _, jobID := range jobIDs {
		w := httptest.NewRecorder()
		r := authedRequest(http.MethodPost, fmt.Sprintf("/v1/jobs/%s/trigger", jobID), `{}`)
		r.Header.Set("X-Idempotency-Key", "universal-key")
		srv.ServeHTTP(w, r)

		if w.Code != http.StatusCreated {
			t.Fatalf("job %s: expected 201, got %d", jobID, w.Code)
		}

		var resp map[string]any
		mustUnmarshal(t, w.Body.Bytes(), &resp)
		responseIDs[jobID] = fmt.Sprintf("%v", resp["id"])
	}

	// Each job should have a unique run ID
	seen := make(map[string]bool)
	for jobID, runID := range responseIDs {
		if seen[runID] {
			t.Fatalf("job %s shares run ID %s with another job", jobID, runID)
		}
		seen[runID] = true
	}
}

// Helper.

func mustUnmarshal(t testing.TB, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("invalid JSON: %v\nbody: %s", err, string(data))
	}
}
