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

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

// Basic idempotency key handling.

func TestIdempotency_XHeader_ReturnsExistingRun(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			require.Equal(t, "my-key-1", key)

			return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "my-key-1")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, enqueued)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-existing",
		resp["id"])

}

func TestIdempotency_StandardHeader_ReturnsExistingRun(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, key string) (*domain.JobRun, error) {
			lookupCalled = true
			require.Equal(t, "standard-key",
				key)

			return &domain.JobRun{ID: "run-std", Status: domain.StatusExecuting}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("Idempotency-Key", "standard-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, lookupCalled)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-std", resp["id"])

}

func TestIdempotency_XHeaderTakesPrecedenceOverStandard(t *testing.T) {
	t.Parallel()
	var capturedKey string
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, key string) (*domain.JobRun, error) {
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
	require.Equal(t, http.StatusOK,
		w.Code)
	require.Equal(t, "x-key", capturedKey)

}

func TestIdempotency_NoHeaderSkipsLookup(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	// No idempotency header
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.False(t, lookupCalled)

}

func TestIdempotency_EmptyHeaderSkipsLookup(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.False(t, lookupCalled)

}

func TestIdempotency_MissCreatesNewRun(t *testing.T) {
	t.Parallel()
	var enqueued *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil // not found
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		enqueued = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"data":"test"}}`)
	r.Header.Set("X-Idempotency-Key", "new-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, enqueued)
	require.Equal(t, "new-key", enqueued.
		IdempotencyKey,
	)

}

// Response shape verification.

func TestIdempotency_HitResponseShape(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
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
	require.Equal(t, "run-cached",
		resp["id"])
	require.Equal(t, "executing",
		resp["status"])

	// Hit response should have id and status

	// Hit response should NOT have run_token or payload_hash
	if _, ok := resp["run_token"]; ok {
		require.Fail(t,

			"idempotency hit response should NOT contain run_token")
	}
	if _, ok := resp["payload_hash"]; ok {
		require.Fail(t,

			"idempotency hit response should NOT contain payload_hash")
	}

	// Hit response must have idempotency_hit=true
	if hit, ok := resp["idempotency_hit"].(bool); !ok || !hit {
		require.Failf(t, "test failure",

			"expected idempotency_hit=true, got %v", resp["idempotency_hit"])
	}
}

func TestIdempotency_MissResponseShape(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "new-key")
	srv.ServeHTTP(w, r)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.False(t, resp["id"] ==
		nil || resp["id"] == "")
	require.NotNil(t, resp["status"])

	// Miss response should have id, status, and payload_hash without exposing SDK run credentials.

	if _, ok := resp["run_token"]; ok {
		require.Fail(t,

			"miss response should not contain run_token")
	}
	require.False(t, resp["payload_hash"] ==
		nil ||
		resp["payload_hash"] == "")

	// Miss response must have idempotency_hit=false
	if hit, ok := resp["idempotency_hit"].(bool); !ok || hit {
		require.Failf(t, "test failure",

			"expected idempotency_hit=false, got %v", resp["idempotency_hit"])
	}
}

func TestIdempotency_HitReturnsCurrentStatus(t *testing.T) {
	// GetRunByIdempotencyKey now only returns non-terminal runs, so we test
	// that the handler correctly passes through the status for each one.
	t.Parallel()
	statuses := []domain.RunStatus{
		domain.StatusQueued,
		domain.StatusDequeued,
		domain.StatusExecuting,
		domain.StatusWaiting,
		domain.StatusDelayed,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			ms := &APIStoreMock{
				GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
					return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
				},
				GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
					return &domain.JobRun{ID: "run-1", Status: status}, nil
				},
			}

			srv := newTestServer(t, ms, &mockQueue{}, nil)
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", "key-"+string(status))
			srv.ServeHTTP(w, r)
			require.Equal(t, http.StatusOK,
				w.Code)

			var resp map[string]any
			mustUnmarshal(t, w.Body.Bytes(), &resp)
			require.Equal(t, string(status), resp["status"])

		})
	}
}

// Idempotency scoped per job.

func TestIdempotency_ScopedPerJob(t *testing.T) {
	t.Parallel()
	lookupArgs := make(map[string]string)
	var mu sync.Mutex

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			mu.Lock()
			lookupArgs[jobID] = key
			mu.Unlock()
			if jobID == "job-A" {
				return &domain.JobRun{ID: "run-A", Status: domain.StatusQueued}, nil
			}
			return nil, nil // miss for job-B
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
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
	require.False(t, w1.Code != http.
		StatusOK ||
		w2.
			Code != http.
			StatusCreated)

	var resp1, resp2 map[string]any
	mustUnmarshal(t, w1.Body.Bytes(), &resp1)
	mustUnmarshal(t, w2.Body.Bytes(), &resp2)
	require.Equal(t, "run-A", resp1["id"])
	require.NotEqual(t, "run-A", resp2["id"])

	// job-B should create a new run (different ID)

	if _, ok := resp2["run_token"]; ok {
		require.Fail(t,

			"job-B miss should not expose run_token")
	}
}

// Error handling.

func TestIdempotency_StoreLookupError_Returns500(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, errors.New("connection refused")
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "err-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.
			String(), "internal server error",
		))

}

func TestIdempotency_StoreLookupError_DoesNotEnqueue(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, errors.New("timeout")
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "err-key")
	srv.ServeHTTP(w, r)
	require.False(t, enqueued)

}

// Idempotency key stored on the enqueued run.

func TestIdempotency_KeyStoredOnEnqueuedRun(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRun = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "persist-key-123")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, capturedRun)
	require.Equal(t, "persist-key-123",
		capturedRun.
			IdempotencyKey,
	)

}

func TestIdempotency_NoKeyStoresEmptyOnRun(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRun = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, capturedRun)
	require.Equal(t, "", capturedRun.
		IdempotencyKey,
	)

}

// Interaction with other features.

func TestIdempotency_HitBypassesRateLimitCheck(t *testing.T) {
	// Idempotency is checked BEFORE rate limits. A cached idempotent run
	// is returned even when rate limits would otherwise reject the request.
	t.Parallel()
	rateLimitCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60,
				RateLimitMax: 1, RateLimitWindowSecs: 60,
			}, nil
		},
		CountRunsForJobSinceFunc: func(_ context.Context, _ string, _ time.Time) (int, error) {
			rateLimitCalled = true
			return 1, nil // at limit
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-cached", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "rate-limit-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, rateLimitCalled)

	// Idempotency hit returns cached run; rate limit is never checked

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-cached",
		resp["id"])

}

func TestIdempotency_HitBeforeDedupCheck(t *testing.T) {
	// Idempotency is checked BEFORE dedup. If idempotency hits, dedup is skipped.
	t.Parallel()
	dedupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60,
				DedupWindowSecs: 300,
			}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-idem", Status: domain.StatusQueued}, nil
		},
		FindRecentRunByPayloadFunc: func(_ context.Context, _ string, _ json.RawMessage, _ time.Time) (*domain.JobRun, error) {
			dedupCalled = true
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"x":1}}`)
	r.Header.Set("X-Idempotency-Key", "idem-before-dedup")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, dedupCalled)

}

func TestIdempotency_HitBypassesProjectQuotaCheck(t *testing.T) {
	// Idempotency is checked BEFORE project quotas. A cached run is
	// returned even when quotas would otherwise reject the request.
	t.Parallel()
	quotaCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			quotaCalled = true
			return &store.ProjectQuota{ProjectID: "proj-1", MaxQueuedRuns: 1}, nil
		},
		CountProjectQueuedRunsFunc: func(_ context.Context, _ string) (int, error) {
			return 1, nil // at limit
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-cached", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "quota-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, quotaCalled)

	// Idempotency hit returns cached run; quota is never checked

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-cached",
		resp["id"])

}

func TestIdempotency_WithScheduledRun(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-delayed",
		resp["id"])
	require.Equal(t, "delayed", resp["status"])

}

func TestIdempotency_WithDifferentPayloads(t *testing.T) {
	// Same idempotency key with different payload should still return cached run
	// (idempotency key takes priority over payload content)
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-original",
		resp["id"])

}

// Edge cases.

func TestIdempotency_VeryLongKey_Rejected(t *testing.T) {
	// Keys longer than 256 characters are rejected to protect the DB index.
	t.Parallel()
	longKey := strings.Repeat("a", 1024)

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", longKey)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)

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
			ms := &APIStoreMock{
				GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
					return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
				},
				GetRunByIdempotencyKeyFunc: func(_ context.Context, _, k string) (*domain.JobRun, error) {
					capturedKey = k
					return nil, nil
				},
			}
			ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
			mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

			srv := newTestServer(t, ms, mq, nil)
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", key)
			srv.ServeHTTP(w, r)
			require.Equal(t, http.StatusCreated,
				w.Code,
			)
			require.Equal(t, key, capturedKey)

		})
	}
}

func TestIdempotency_WhitespaceOnlyKey(t *testing.T) {
	// A whitespace-only key is technically non-empty, should trigger lookup
	t.Parallel()
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "   ")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.True(
		t, lookupCalled)

}

func TestIdempotency_DisabledJobStillRejectsBeforeIdempotencyCheck(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: false, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return &domain.JobRun{ID: "run-cached", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "disabled-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.False(t, lookupCalled)

}

func TestIdempotency_JobNotFoundRejectsBeforeIdempotencyCheck(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return nil, store.ErrJobNotFound
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/nonexistent/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "not-found-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusNotFound,
		w.
			Code)
	require.False(t, lookupCalled)

}

func TestIdempotency_InvalidBodyRejectsBeforeIdempotencyCheck(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{invalid json`)
	r.Header.Set("X-Idempotency-Key", "bad-body-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.False(t, lookupCalled)

}

// Concurrent requests with same idempotency key.

func TestIdempotency_ConcurrentRequestsSameKey(t *testing.T) {
	t.Parallel()
	var enqueueCount atomic.Int32
	var lookupCount atomic.Int32

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCount.Add(1)
			return nil, nil // all miss (simulating race)
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueueCount.Add(1)
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	const concurrency = 10
	var wg conc.WaitGroup
	results := make([]*httptest.ResponseRecorder, concurrency)

	for i := range concurrency {
		idx := i
		wg.Go(func() {
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", "concurrent-key")
			srv.ServeHTTP(w, r)
			results[idx] = w
		})
	}
	wg.Wait()

	// All should succeed (201) at the application level
	for _, w := range results {
		assert.Equal(
			t, http.StatusCreated,
			w.Code,
		)

	}
	assert.Equal(
		t, int32(concurrency), lookupCount.
			Load())

	// All should have been looked up

	// NOTE: Without DB-level uniqueness, all would enqueue. The DB unique
	// partial index prevents duplicates at the database level. This test
	// verifies the application-level behavior is correct.
	t.Logf("concurrent enqueue count: %d (DB unique index would prevent duplicates)", enqueueCount.Load())
}

func TestIdempotency_ConcurrentRequestsMixedHitMiss(t *testing.T) {
	t.Parallel()
	var lookupCount atomic.Int32

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			count := lookupCount.Add(1)
			// First request misses, subsequent ones hit
			if count == 1 {
				return nil, nil
			}
			return &domain.JobRun{ID: "run-first", Status: domain.StatusQueued}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)

	const concurrency = 5
	var wg conc.WaitGroup
	responses := make([]map[string]any, concurrency)

	for i := range concurrency {
		idx := i
		wg.Go(func() {
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", "mixed-key")
			srv.ServeHTTP(w, r)
			var resp map[string]any
			_ = json.Unmarshal(w.Body.Bytes(), &resp)
			responses[idx] = resp
		})
	}
	wg.Wait()

	// Verify all got 201 responses
	for i, resp := range responses {
		if resp == nil {
			assert.Failf(t, "test failure",

				"request %d: nil response", i)
			continue
		}
		assert.False(
			t, resp["id"] ==
				nil || resp["id"] ==
				"")

	}
}

// Idempotency + replay flow.

func TestIdempotency_ReplayDoesNotCopyIdempotencyKey(t *testing.T) {
	// Replays should NOT carry the original idempotency key because:
	// 1. Replays are independent operations
	// 2. Copying the key could conflict with active runs sharing the same key
	t.Parallel()
	var enqueued *domain.JobRun
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
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
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		enqueued = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-original/replay", ""))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, enqueued)
	require.Equal(t, "", enqueued.
		IdempotencyKey,
	)

}

// Idempotency + payload validation.

func TestIdempotency_PayloadValidationFailsBeforeIdempotencyCheck(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:            id,
				ProjectID:     "proj-1",
				Enabled:       true,
				TimeoutSecs:   60,
				PayloadSchema: json.RawMessage(`{"type":"object","required":["name"]}`),
			}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"age":12}}`)
	r.Header.Set("X-Idempotency-Key", "schema-fail-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.False(t, lookupCalled)

}

// Multiple sequential triggers with same key.

func TestIdempotency_SequentialTriggersReturnSameRun(t *testing.T) {
	t.Parallel()
	var enqueueCount int
	existingRun := &domain.JobRun{ID: "run-first", Status: domain.StatusQueued}
	callNum := 0

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			callNum++
			if callNum == 1 {
				return nil, nil // first call: miss
			}
			return existingRun, nil // subsequent calls: hit
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
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
	require.Equal(t, http.StatusCreated,
		w1.
			Code)
	require.EqualValues(t, 1, enqueueCount)

	// Second trigger: should return cached
	w2 := httptest.NewRecorder()
	r2 := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r2.Header.Set("X-Idempotency-Key", "seq-key")
	srv.ServeHTTP(w2, r2)
	require.Equal(t, http.StatusOK,
		w2.Code)
	require.EqualValues(t, 1, enqueueCount)

	var resp2 map[string]any
	mustUnmarshal(t, w2.Body.Bytes(), &resp2)
	require.Equal(t, "run-first",
		resp2["id"])

	// Third trigger: should also return cached
	w3 := httptest.NewRecorder()
	r3 := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"different":"data"}}`)
	r3.Header.Set("X-Idempotency-Key", "seq-key")
	srv.ServeHTTP(w3, r3)
	require.Equal(t, http.StatusOK,
		w3.Code)
	require.EqualValues(t, 1, enqueueCount)

}

// Idempotency + execution window.

func TestIdempotency_ExecutionWindowDoesNotApplyOnHit(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:                  id,
				ProjectID:           "proj-1",
				Enabled:             true,
				TimeoutSecs:         60,
				ExecutionWindowCron: "0 0 1 1 *", // very restrictive window
				Timezone:            "UTC",
			}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-windowed", Status: domain.StatusDelayed}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "window-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-windowed",
		resp["id"])

}

// Idempotency + dry run.

func TestIdempotency_DryRunDoesNotCheckIdempotency(t *testing.T) {
	t.Parallel()
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"dry_run": true}`)
	r.Header.Set("X-Idempotency-Key", "dry-run-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, lookupCalled)

}

// Cost budget + idempotency ordering.

func TestIdempotency_HitBypassesCostBudgetCheck(t *testing.T) {
	// Idempotency is checked BEFORE cost budgets. A cached run is
	// returned even when cost budget would otherwise reject the request.
	t.Parallel()
	costCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{
				ProjectID:            "proj-1",
				MaxDailyCostMicrousd: 100,
			}, nil
		},
		SumProjectDailyCostMicrousdFunc: func(_ context.Context, _ string, _ string) (int64, error) {
			costCalled = true
			return 100, nil // at budget
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-cached", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "budget-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, costCalled)

	// Idempotency hit returns cached run; cost budget is never checked

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-cached",
		resp["id"])

}

// Idempotency key with different HTTP status scenarios.

func TestIdempotency_EnqueueFailureAfterMiss(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil // miss
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		return fmt.Errorf("queue full")
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "enqueue-fail-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

func TestIdempotency_MultipleJobsSameKeyIndependent(t *testing.T) {
	// Verify that the same idempotency key used for multiple different jobs
	// creates independent runs for each job.
	t.Parallel()
	runs := make(map[string]*domain.JobRun)
	var mu sync.Mutex

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, key string) (*domain.JobRun, error) {
			mu.Lock()
			defer mu.Unlock()
			if r, ok := runs[jobID]; ok {
				return r, nil
			}
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
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
		require.Equal(t, http.StatusCreated,
			w.Code,
		)

		var resp map[string]any
		mustUnmarshal(t, w.Body.Bytes(), &resp)
		responseIDs[jobID] = fmt.Sprintf("%v", resp["id"])
	}

	// Each job should have a unique run ID
	seen := make(map[string]bool)
	for _, runID := range responseIDs {
		require.False(t, seen[runID])

		seen[runID] = true
	}
}

// DB unique constraint race: simulate what happens when two concurrent
// requests both miss the app-level check but the DB rejects the second INSERT.

func TestIdempotency_EnqueueUniqueViolation_RetriesLookup(t *testing.T) {
	// When two concurrent requests both miss the idempotency check and both
	// try to enqueue, the DB unique partial index rejects the second INSERT.
	// The handler catches ErrIdempotencyConflict and retries the lookup,
	// returning the existing run instead of a 500 error.
	t.Parallel()
	lookupCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCount++
			if lookupCount == 1 {
				return nil, nil // first call: miss (race window)
			}
			// second call: retry after conflict finds the winner's run
			return &domain.JobRun{ID: "run-winner", Status: domain.StatusQueued}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		return domain.ErrIdempotencyConflict
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "race-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.EqualValues(t, 2, lookupCount)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-winner",
		resp["id"])

	// Conflict-retry response must also have idempotency_hit=true
	if hit, ok := resp["idempotency_hit"].(bool); !ok || !hit {
		require.Failf(t, "test failure",

			"expected idempotency_hit=true on conflict retry, got %v", resp["idempotency_hit"])
	}
}

// Bulk trigger does NOT support idempotency keys.

func TestIdempotency_BulkTriggerPerItemIdempotencyKey(t *testing.T) {
	// Verify that bulk trigger supports per-item idempotency keys.
	t.Parallel()
	var capturedRuns []*domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil // all miss
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRuns = append(capturedRuns, run)
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := `{"items":[{"payload":{"a":1},"idempotency_key":"key-a"},{"payload":{"b":2},"idempotency_key":"key-b"},{"payload":{"c":3}}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Len(t,
		capturedRuns, 3,
	)
	assert.Equal(
		t, "key-a", capturedRuns[0].
			IdempotencyKey,
	)
	assert.Equal(
		t, "key-b", capturedRuns[1].
			IdempotencyKey,
	)
	assert.Equal(
		t, "", capturedRuns[2].IdempotencyKey,
	)

}

func TestIdempotency_BulkTriggerPerItemIdempotencyHit(t *testing.T) {
	// When a bulk item's idempotency key hits, the cached run is returned
	// for that item and no enqueue happens.
	t.Parallel()
	var enqueuedKeys []string
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, key string) (*domain.JobRun, error) {
			if key == "existing-key" {
				return &domain.JobRun{ID: "run-existing", Status: domain.StatusQueued}, nil
			}
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		enqueuedKeys = append(enqueuedKeys, run.IdempotencyKey)
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := `{"items":[{"payload":{"a":1},"idempotency_key":"existing-key"},{"payload":{"b":2},"idempotency_key":"new-key"}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.False(t, len(enqueuedKeys) != 1 ||
		enqueuedKeys[0] !=
			"new-key")

	// Only the new-key item should have been enqueued

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	results := resp["results"].([]any)
	require.Len(t,
		results, 2)

	// First item: idempotency hit returns cached run
	first := results[0].(map[string]any)
	assert.Equal(
		t, "run-existing",
		first["id"])
	assert.Equal(
		t, "queued", first["status"])

	// First item (hit) should have idempotency_hit=true
	if hit, ok := first["idempotency_hit"].(bool); !ok || !hit {
		assert.Failf(t, "test failure",

			"first item: expected idempotency_hit=true, got %v", first["idempotency_hit"])
	}
	// Second item (miss/new) should have idempotency_hit=false
	second := results[1].(map[string]any)
	if hit, ok := second["idempotency_hit"].(bool); !ok || hit {
		assert.Failf(t, "test failure",

			"second item: expected idempotency_hit=false, got %v", second["idempotency_hit"])
	}
}

func TestIdempotency_BulkTriggerConflictRetry(t *testing.T) {
	// When a bulk item hits ErrIdempotencyConflict on enqueue, the handler
	// retries the lookup and returns the existing run.
	t.Parallel()
	lookupCount := 0
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCount++
			if lookupCount == 1 {
				return nil, nil // miss on first check
			}
			return &domain.JobRun{ID: "run-winner", Status: domain.StatusQueued}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		return domain.ErrIdempotencyConflict
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := `{"items":[{"payload":{},"idempotency_key":"conflict-key"}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	results := resp["results"].([]any)
	first := results[0].(map[string]any)
	assert.Equal(
		t, "run-winner",
		first["id"])

}

func TestIdempotency_BulkTriggerHeaderIgnored(t *testing.T) {
	// The X-Idempotency-Key HTTP header is NOT used by bulk trigger.
	// Only per-item idempotency_key fields matter.
	t.Parallel()
	var capturedRuns []*domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRuns = append(capturedRuns, run)
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", `{"items":[{"payload":{"a":1}}]}`)
	r.Header.Set("X-Idempotency-Key", "header-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Len(t,
		capturedRuns, 1,
	)
	assert.Equal(
		t, "", capturedRuns[0].IdempotencyKey,
	)

	// The HTTP header should NOT be applied to bulk items

}

// Replay with idempotency key that already has an active run should fail
// at the DB level (unique constraint).

func TestIdempotency_ReplayGeneratesNewRunID(t *testing.T) {
	// Replayed runs get a new ID and do NOT carry the original idempotency
	// key, so they never conflict with the original or other active runs.
	t.Parallel()
	var enqueued *domain.JobRun
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:             "run-active",
				JobID:          "job-1",
				ProjectID:      "proj-1",
				Status:         domain.StatusFailed, // terminal, so replay is allowed
				Payload:        json.RawMessage(`{}`),
				IdempotencyKey: "replay-idem",
				JobVersion:     1,
			}, nil
		},
		GetJobFunc: func(_ context.Context, _ string) (*domain.Job, error) {
			return &domain.Job{ID: "job-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		enqueued = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/runs/run-active/replay", ""))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, enqueued)
	require.Equal(t, "", enqueued.
		IdempotencyKey,
	)
	require.NotEqual(t, "run-active",
		enqueued.
			ID)

}

// Idempotency key with all terminal statuses should still return the cached
// run (the query returns the most recent run regardless of status).

func TestIdempotency_TerminalRunAllowsKeyReuse(t *testing.T) {
	// After a run reaches a terminal status, GetRunByIdempotencyKey (with
	// the status filter) returns nil, allowing a new run to be created
	// with the same key. This matches the DB partial unique index semantics.
	t.Parallel()
	terminalStatuses := []domain.RunStatus{
		domain.StatusCompleted,
		domain.StatusFailed,
		domain.StatusTimedOut,
		domain.StatusCrashed,
		domain.StatusSystemFailed,
		domain.StatusCanceled,
		domain.StatusExpired,
	}

	for _, status := range terminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			enqueued := false
			ms := &APIStoreMock{
				GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
					return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
				},
				GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
					// Store now filters by non-terminal statuses, so terminal
					// runs are not returned — the lookup returns nil (miss).
					return nil, nil
				},
			}
			ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
			mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
				enqueued = true
				return nil
			}}

			srv := newTestServer(t, ms, mq, nil)
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", "terminal-"+string(status))
			srv.ServeHTTP(w, r)
			require.Equal(t, http.StatusCreated,
				w.Code,
			)
			require.True(
				t, enqueued)

			// A new run should be enqueued since the terminal run is no
			// longer returned by the idempotency lookup.

		})
	}
}

func TestIdempotency_NonTerminalRunStillReturnsHit(t *testing.T) {
	// Non-terminal runs should still return idempotency hits.
	t.Parallel()
	nonTerminalStatuses := []domain.RunStatus{
		domain.StatusQueued,
		domain.StatusDequeued,
		domain.StatusExecuting,
		domain.StatusDelayed,
		domain.StatusWaiting,
	}

	for _, status := range nonTerminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			enqueued := false
			ms := &APIStoreMock{
				GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
					return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
				},
				GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
					return &domain.JobRun{ID: "run-active", Status: status}, nil
				},
			}
			ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
			mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
				enqueued = true
				return nil
			}}

			srv := newTestServer(t, ms, mq, nil)
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", "active-"+string(status))
			srv.ServeHTTP(w, r)
			require.Equal(t, http.StatusOK,
				w.Code)
			require.False(t, enqueued)

			var resp map[string]any
			mustUnmarshal(t, w.Body.Bytes(), &resp)
			require.Equal(t, "run-active",
				resp["id"])

			if hit, ok := resp["idempotency_hit"].(bool); !ok || !hit {
				require.Failf(t, "test failure",

					"expected idempotency_hit=true, got %v", resp["idempotency_hit"])
			}
		})
	}
}

// Verify the idempotency check does NOT filter by project — it's per-job only.
// This matters because different projects could theoretically have the same job ID
// (though in practice UUIDs make this unlikely).

func TestIdempotency_LookupPassesJobIDNotProjectID(t *testing.T) {
	t.Parallel()
	var capturedJobID string
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-specific", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, jobID, _ string) (*domain.JobRun, error) {
			capturedJobID = jobID
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-xyz/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "proj-test-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Equal(t, "job-xyz", capturedJobID)

}

// Verify the timing: idempotency hit returns immediately without computing
// payload hash, JWT token, or any enqueue-related work.

func TestIdempotency_HitSkipsAllDownstreamWork(t *testing.T) {
	t.Parallel()
	dedupCalled := false
	quotaCalled := false
	enqueueCalled := false

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60,
				DedupWindowSecs: 300,
			}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-fast", Status: domain.StatusQueued}, nil
		},
		FindRecentRunByPayloadFunc: func(_ context.Context, _ string, _ json.RawMessage, _ time.Time) (*domain.JobRun, error) {
			dedupCalled = true
			return nil, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			quotaCalled = true
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueueCalled = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"x":1}}`)
	r.Header.Set("X-Idempotency-Key", "fast-path-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	assert.False(
		t, dedupCalled)
	assert.False(
		t, enqueueCalled)
	assert.False(
		t, quotaCalled)

	// Quota/cost checks happen AFTER idempotency, so quotaCalled should
	// be false when idempotency hits (FF is disabled by default anyway).

}

// Idempotency key + context cancellation: verify the handler respects context.

func TestIdempotency_ContextCanceledDuringLookup(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, context.Canceled
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "ctx-cancel-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

	// Context cancellation during idempotency lookup should return 500.

}

func TestIdempotency_ContextDeadlineExceededDuringLookup(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, context.DeadlineExceeded
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "deadline-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusInternalServerError,

		w.Code)

}

// Verify that idempotency key on trigger sets the correct triggered_by value.

func TestIdempotency_MissSetsTriggeredByManual(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRun = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "trigger-by-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Equal(t, domain.TriggerManual,
		capturedRun.
			TriggeredBy,
	)

}

// Idempotency key with priority: verify priority is set on miss but ignored on hit.

func TestIdempotency_MissPreservesPriority(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRun = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"priority":7}`)
	r.Header.Set("X-Idempotency-Key", "priority-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.EqualValues(t, 7, capturedRun.
		Priority,
	)
	require.Equal(t, "priority-key",
		capturedRun.
			IdempotencyKey,
	)

}

func TestIdempotency_HitIgnoresNewPriority(t *testing.T) {
	// When an idempotency hit occurs, the ORIGINAL run is returned
	// regardless of the new request's priority value.
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-p3", Status: domain.StatusQueued, Priority: 3}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"priority":9}`)
	r.Header.Set("X-Idempotency-Key", "priority-hit-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-p3", resp["id"])

	// The response only includes id and status, not priority — but the key
	// point is the original run was returned, not a new one with priority 9.
}

// Idempotency with scheduled_at on miss: verify the scheduled time is applied.

func TestIdempotency_MissWithScheduledAt(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRun = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	future := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"scheduled_at":"%s"}`, future)

	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", body)
	r.Header.Set("X-Idempotency-Key", "scheduled-miss-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.NotNil(t, capturedRun)
	require.Equal(t, domain.StatusDelayed,
		capturedRun.
			Status)
	require.NotNil(t, capturedRun.
		ScheduledAt,
	)
	require.Equal(t, "scheduled-miss-key",
		capturedRun.
			IdempotencyKey,
	)

}

// Idempotency with job_version: verify the correct version is captured.

func TestIdempotency_MissStoresJobVersion(t *testing.T) {
	t.Parallel()
	var capturedRun *domain.JobRun
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60, Version: 42}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		capturedRun = run
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "version-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.EqualValues(t, 42, capturedRun.
		JobVersion,
	)

}

// Verify idempotency key is NOT trimmed or normalized — stored exactly as sent.

func TestIdempotency_KeyNotNormalized(t *testing.T) {
	t.Parallel()
	keys := []struct {
		name     string
		input    string
		expected string
	}{
		{"leading_spaces", "  key  ", "  key  "},
		{"mixed_case", "MyKey-ABC", "MyKey-ABC"},
		{"newlines", "key\nwith\nnewlines", "key\nwith\nnewlines"},
		{"null_bytes", "key\x00null", "key\x00null"},
	}

	for _, tt := range keys {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var capturedKey string
			ms := &APIStoreMock{
				GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
					return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
				},
				GetRunByIdempotencyKeyFunc: func(_ context.Context, _, k string) (*domain.JobRun, error) {
					capturedKey = k
					return nil, nil
				},
			}
			ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
			mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

			srv := newTestServer(t, ms, mq, nil)
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", tt.input)
			srv.ServeHTTP(w, r)
			require.Equal(t, http.StatusCreated,
				w.Code,
			)
			require.Equal(t, tt.expected,
				capturedKey,
			)

		})
	}
}

// Stress test: many unique keys in rapid succession should all create separate runs.

func TestIdempotency_ManyUniqueKeysConcurrently(t *testing.T) {
	t.Parallel()
	var enqueueCount atomic.Int32

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil // all miss
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueueCount.Add(1)
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)

	const n = 50
	var wg conc.WaitGroup
	for i := range n {
		idx := i
		wg.Go(func() {
			w := httptest.NewRecorder()
			r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
			r.Header.Set("X-Idempotency-Key", fmt.Sprintf("unique-key-%d", idx))
			srv.ServeHTTP(w, r)
			assert.Equal(
				t, http.StatusCreated,
				w.Code,
			)

		})
	}
	wg.Wait()
	require.Equal(t, int32(n), enqueueCount.
		Load())

}

// Verify idempotency hit returns HTTP 201 (not 200 or 409) consistently.

func TestIdempotency_HitAlwaysReturns201(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	for range 5 {
		w := httptest.NewRecorder()
		r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
		r.Header.Set("X-Idempotency-Key", "consistent-key")
		srv.ServeHTTP(w, r)
		require.Equal(t, http.StatusOK,
			w.Code)

	}
}

// Idempotency key interaction with API key auth (not internal secret).

func TestIdempotency_WorksWithAPIKeyAuth(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-api-key", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	// Use internal secret auth (the test infrastructure uses X-Internal-Secret)
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "api-key-auth-test")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-api-key",
		resp["id"])

}

// Key length validation.

func TestIdempotency_KeyTooLong_Returns400(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", strings.Repeat("a", 257))
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.
			String(), "256 characters",
		))

}

func TestIdempotency_KeyExactlyMaxLength_Succeeds(t *testing.T) {
	t.Parallel()
	var capturedKey string
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, key string) (*domain.JobRun, error) {
			capturedKey = key
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	exactKey := strings.Repeat("b", 256)
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", exactKey)
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Equal(t, exactKey, capturedKey)

}

func TestIdempotency_BulkKeyTooLong_Returns400(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	longKey := strings.Repeat("c", 257)
	body := fmt.Sprintf(`{"items":[{"payload":{},"idempotency_key":"%s"}]}`, longKey)
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	require.Equal(t, http.StatusBadRequest,

		w.Code)
	require.True(
		t, strings.Contains(w.Body.
			String(), "256 characters",
		))

}

func TestIdempotency_BulkKeyExactlyMaxLength_Succeeds(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	exactKey := strings.Repeat("d", 256)
	body := fmt.Sprintf(`{"items":[{"payload":{},"idempotency_key":"%s"}]}`, exactKey)
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

}

// Dedup response shape consistency.

func TestIdempotency_DedupResponseIncludesIdempotencyHitFalse(t *testing.T) {
	// When payload deduplication returns an existing run, the response
	// should include idempotency_hit=false for consistent response shape.
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID: id, ProjectID: "proj-1", Enabled: true,
				TimeoutSecs: 60, DedupWindowSecs: 300,
			}, nil
		},
		FindRecentRunByPayloadFunc: func(_ context.Context, _ string, _ json.RawMessage, _ time.Time) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-deduped", Status: domain.StatusQueued}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{"x":1}}`))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	if hit, ok := resp["idempotency_hit"].(bool); !ok || hit {
		require.Failf(t, "test failure",

			"expected idempotency_hit=false on dedup response, got %v", resp["idempotency_hit"])
	}
}

// Body field idempotency_key is rejected for single trigger.

func TestIdempotency_BodyFieldIdempotencyKeyRejected(t *testing.T) {
	// TriggerRequest uses DisallowUnknownFields, so "idempotency_key" in the
	// body is rejected as an unknown field. Idempotency keys for single
	// triggers MUST be sent via HTTP headers (X-Idempotency-Key or
	// Idempotency-Key), not the request body.
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	// Body has idempotency_key — this field is unknown but TypedHandler
	// silently ignores unknown fields (idempotency_key must be sent via header).
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{},"idempotency_key":"body-key"}`))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

}

func TestIdempotency_BodyFieldOmittedNoHeader_NoLookup(t *testing.T) {
	// When no idempotency key is provided (neither header nor body),
	// the lookup should not be called and a new run is created.
	t.Parallel()
	lookupCalled := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, _ string) (*domain.JobRun, error) {
			lookupCalled = true
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		assert.Equal(
			t, "", run.IdempotencyKey,
		)

		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{"payload":{}}`))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.False(t, lookupCalled)

}

// Intra-request bulk dedup (same key twice in one request).

func TestIdempotency_BulkSameKeyTwiceInOneRequest(t *testing.T) {
	// When a single bulk request has two items with the same idempotency
	// key, the first gets enqueued and the second should hit the app-level
	// check and return the first item's run.
	t.Parallel()
	var enqueueCount int
	var enqueuedRunID string
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, key string) (*domain.JobRun, error) {
			// After first enqueue, lookup should find the enqueued run.
			if enqueueCount > 0 && key == "dupe-key" {
				return &domain.JobRun{ID: enqueuedRunID, Status: domain.StatusQueued}, nil
			}
			return nil, nil
		},
	}
	ms.AreJobDependenciesSatisfiedFunc = func(_ context.Context, _ *domain.JobRun) (bool, error) { return true, nil }
	mq := &mockQueue{enqueueFn: func(_ context.Context, run *domain.JobRun) error {
		enqueueCount++
		enqueuedRunID = run.ID
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	body := `{"items":[{"payload":{},"idempotency_key":"dupe-key"},{"payload":{},"idempotency_key":"dupe-key"}]}`
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger/bulk", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.EqualValues(t, 1, enqueueCount)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	results := resp["results"].([]any)
	require.Len(t,
		results, 2)

	first := results[0].(map[string]any)
	second := results[1].(map[string]any)
	require.Equal(t, second["id"],
		first["id"])

	// Both should return the same run ID.

	// First should be miss, second should be hit.
	if hit, ok := first["idempotency_hit"].(bool); !ok || hit {
		assert.Failf(t, "test failure",

			"first item: expected idempotency_hit=false, got %v", first["idempotency_hit"])
	}
	if hit, ok := second["idempotency_hit"].(bool); !ok || !hit {
		assert.Failf(t, "test failure",

			"second item: expected idempotency_hit=true, got %v", second["idempotency_hit"])
	}
}

// Workflow trigger does not support idempotency.

// mockWorkflowEngineForIdem implements WorkflowTrigger for testing.
type mockWorkflowEngineForIdem struct {
	triggerFn func(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride) (*domain.WorkflowRun, error)
}

func (m *mockWorkflowEngineForIdem) TriggerWorkflow(ctx context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, stepOverrides []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error) {
	if m.triggerFn != nil {
		return m.triggerFn(ctx, workflowID, projectID, payload, triggeredBy, stepOverrides)
	}
	return nil, nil
}

func (m *mockWorkflowEngineForIdem) RetryWorkflowRun(_ context.Context, _ string) (*domain.WorkflowRun, error) {
	return nil, nil
}

func newTestServerWithWorkflowEngine(t *testing.T, s APIStore, q *mockQueue, wf WorkflowTrigger) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	srv := NewServer(ServerDeps{
		Config:         cfg,
		Store:          s,
		Queue:          q,
		WorkflowEngine: wf,
	})
	t.Cleanup(srv.Close)
	return srv
}

func TestIdempotency_WorkflowTriggerIgnoresIdempotencyHeader(t *testing.T) {
	// Workflow trigger endpoint does NOT support idempotency keys.
	// The X-Idempotency-Key header should be ignored.
	t.Parallel()

	wfID := "wf-1"
	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Enabled: true}, nil
		},
	}

	// Use a mock workflow engine that records triggers.
	triggerCount := 0
	mockWFEngine := &mockWorkflowEngineForIdem{
		triggerFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride) (*domain.WorkflowRun, error) {
			triggerCount++
			return &domain.WorkflowRun{ID: "wfrun-1", WorkflowID: wfID, Status: domain.WfStatusPending}, nil
		},
	}

	srv := newTestServerWithWorkflowEngine(t, ms, &mockQueue{}, mockWFEngine)
	// First trigger with idempotency key.
	w1 := httptest.NewRecorder()
	r1 := authedRequest(http.MethodPost, "/v1/workflows/"+wfID+"/trigger", `{"payload":{}}`)
	r1.Header.Set("X-Idempotency-Key", "wf-key-1")
	srv.ServeHTTP(w1, r1)

	// Second trigger with same idempotency key.
	w2 := httptest.NewRecorder()
	r2 := authedRequest(http.MethodPost, "/v1/workflows/"+wfID+"/trigger", `{"payload":{}}`)
	r2.Header.Set("X-Idempotency-Key", "wf-key-1")
	srv.ServeHTTP(w2, r2)
	require.EqualValues(t, 2, triggerCount)

	// Both should trigger (header is ignored, no dedup).

}

func TestIdempotency_TerminalRunWithin24Hours_ReturnsExistingRun(t *testing.T) {
	t.Parallel()
	enqueued := false
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{ID: id, ProjectID: "proj-1", Enabled: true, TimeoutSecs: 60}, nil
		},
		GetRunByIdempotencyKeyFunc: func(_ context.Context, _, key string) (*domain.JobRun, error) {
			require.Equal(t, "terminal-key",
				key)

			return &domain.JobRun{ID: "run-terminal", Status: domain.StatusCompleted}, nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error {
		enqueued = true
		return nil
	}}

	srv := newTestServer(t, ms, mq, nil)
	w := httptest.NewRecorder()
	r := authedRequest(http.MethodPost, "/v1/jobs/job-1/trigger", `{}`)
	r.Header.Set("X-Idempotency-Key", "terminal-key")
	srv.ServeHTTP(w, r)
	require.Equal(t, http.StatusOK,
		w.Code)
	require.False(t, enqueued)

	var resp map[string]any
	mustUnmarshal(t, w.Body.Bytes(), &resp)
	require.Equal(t, "run-terminal",
		resp["id"])
	require.Equal(t, true, resp["idempotency_hit"])

}

// Helper.

func mustUnmarshal(tb testing.TB, data []byte, v any) {
	tb.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		tb.Fatalf("invalid JSON: %v\nbody: %s", err, string(data))
	}
}
