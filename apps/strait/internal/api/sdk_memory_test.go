package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

func TestHandleSDKSetMemory_SuccessUsesAtomicQuotaUpsert(t *testing.T) {
	t.Parallel()

	var (
		captured  *domain.JobMemory
		maxPerKey int
		maxPerJob int
	)
	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1"}, nil
		},
		getProjectQuotaFn: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			if projectID != "proj-1" {
				t.Fatalf("unexpected projectID %s", projectID)
			}
			return &store.ProjectQuota{MaxMemoryPerKeyBytes: 128, MaxMemoryPerJobBytes: 512}, nil
		},
		upsertJobMemoryWithQuotaFn: func(_ context.Context, mem *domain.JobMemory, perKey, perJob int) error {
			captured = mem
			maxPerKey = perKey
			maxPerJob = perJob
			mem.ID = "mem-1"
			return nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/memory/cache-key", "run-1", `{"value":{"ok":true},"ttl_secs":60}`)
	chi.RouteContext(r.Context()).URLParams.Add("key", "cache-key")
	srv.handleSDKSetMemory(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured == nil {
		t.Fatal("expected UpsertJobMemoryWithQuota to be called")
	}
	if captured.JobID != "job-1" || captured.ProjectID != "proj-1" || captured.MemoryKey != "cache-key" {
		t.Fatalf("unexpected memory payload: %+v", captured)
	}
	if captured.SizeBytes == 0 {
		t.Fatal("expected non-zero size bytes")
	}
	if captured.TTLExpiresAt == nil {
		t.Fatal("expected TTL expiry to be set")
	}
	if maxPerKey != 128 || maxPerJob != 512 {
		t.Fatalf("unexpected quota args: perKey=%d perJob=%d", maxPerKey, maxPerJob)
	}
}

func TestHandleSDKSetMemory_PerKeyQuotaExceeded(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1"}, nil
		},
		getProjectQuotaFn: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{MaxMemoryPerKeyBytes: 4, MaxMemoryPerJobBytes: 512}, nil
		},
		upsertJobMemoryWithQuotaFn: func(_ context.Context, _ *domain.JobMemory, _, _ int) error {
			return store.ErrJobMemoryPerKeyLimitExceeded
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/memory/cache-key", "run-1", `{"value":"hello"}`)
	chi.RouteContext(r.Context()).URLParams.Add("key", "cache-key")
	srv.handleSDKSetMemory(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "value exceeds per-key memory limit") {
		t.Fatalf("expected per-key limit message, got %s", w.Body.String())
	}
}

func TestHandleSDKSetMemory_PerJobQuotaExceeded(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1"}, nil
		},
		getProjectQuotaFn: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{MaxMemoryPerKeyBytes: 128, MaxMemoryPerJobBytes: 8}, nil
		},
		upsertJobMemoryWithQuotaFn: func(_ context.Context, _ *domain.JobMemory, _, _ int) error {
			return store.ErrJobMemoryPerJobLimitExceeded
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/memory/cache-key", "run-1", `{"value":"hello"}`)
	chi.RouteContext(r.Context()).URLParams.Add("key", "cache-key")
	srv.handleSDKSetMemory(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "value exceeds per-job memory limit") {
		t.Fatalf("expected per-job limit message, got %s", w.Body.String())
	}
}
