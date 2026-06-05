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
	"github.com/stretchr/testify/require"
)

func TestHandleSDKSetMemory_SuccessUsesAtomicQuotaUpsert(t *testing.T) {
	t.Parallel()

	var (
		captured  *domain.JobMemory
		maxPerKey int
		maxPerJob int
	)
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1"}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			require.Equal(t, "proj-1",
				projectID)

			return &store.ProjectQuota{MaxMemoryPerKeyBytes: 128, MaxMemoryPerJobBytes: 512}, nil
		},
		UpsertJobMemoryWithQuotaFunc: func(_ context.Context, mem *domain.JobMemory, perKey, perJob int) error {
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
	TypedHandler(srv, http.StatusCreated, srv.handleSDKSetMemory)(w, r)
	require.Equal(t, http.StatusCreated,
		w.Code)
	require.NotNil(t, captured)
	require.False(t, captured.
		JobID != "job-1" ||
		captured.ProjectID !=
			"proj-1" ||
		captured.MemoryKey !=
			"cache-key")
	require.NotEqual(t, 0, captured.
		SizeBytes)
	require.NotNil(t, captured.
		TTLExpiresAt)
	require.False(t, maxPerKey !=
		128 || maxPerJob !=
		512)

}

func TestHandleSDKSetMemory_PerKeyQuotaExceeded(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1"}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{MaxMemoryPerKeyBytes: 4, MaxMemoryPerJobBytes: 512}, nil
		},
		UpsertJobMemoryWithQuotaFunc: func(_ context.Context, _ *domain.JobMemory, _, _ int) error {
			return store.ErrJobMemoryPerKeyLimitExceeded
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/memory/cache-key", "run-1", `{"value":"hello"}`)
	chi.RouteContext(r.Context()).URLParams.Add("key", "cache-key")
	TypedHandler(srv, http.StatusCreated, srv.handleSDKSetMemory)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
	require.True(
		t, strings.Contains(w.Body.String(), "value exceeds per-key memory limit"))

}

func TestHandleSDKSetMemory_PerJobQuotaExceeded(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1"}, nil
		},
		GetProjectQuotaFunc: func(_ context.Context, _ string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{MaxMemoryPerKeyBytes: 128, MaxMemoryPerJobBytes: 8}, nil
		},
		UpsertJobMemoryWithQuotaFunc: func(_ context.Context, _ *domain.JobMemory, _, _ int) error {
			return store.ErrJobMemoryPerJobLimitExceeded
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/memory/cache-key", "run-1", `{"value":"hello"}`)
	chi.RouteContext(r.Context()).URLParams.Add("key", "cache-key")
	TypedHandler(srv, http.StatusCreated, srv.handleSDKSetMemory)(w, r)
	require.Equal(t, http.StatusBadRequest,
		w.Code,
	)
	require.True(
		t, strings.Contains(w.Body.String(), "value exceeds per-job memory limit"))

}
