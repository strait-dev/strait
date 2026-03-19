package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/go-chi/chi/v5"
)

func TestHandleListRunResources_SameProject(t *testing.T) {
	t.Parallel()

	wantFrom := time.Date(2026, 3, 19, 10, 0, 0, 0, time.UTC)
	wantTo := wantFrom.Add(15 * time.Minute)
	wantSnapshots := []domain.RunResourceSnapshot{
		{
			ID:         "snap-1",
			RunID:      "run-1",
			CPUPercent: 42.5,
			CreatedAt:  wantFrom.Add(5 * time.Minute),
		},
	}

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			if id != "run-1" {
				t.Fatalf("unexpected run id %q", id)
			}
			return &domain.JobRun{ID: id, ProjectID: "proj-1"}, nil
		},
		listRunResourceSnapshotsFn: func(_ context.Context, runID string, from, to *time.Time, limit int) ([]domain.RunResourceSnapshot, error) {
			if runID != "run-1" {
				t.Fatalf("unexpected run id %q", runID)
			}
			if from == nil || !from.Equal(wantFrom) {
				t.Fatalf("unexpected from %v", from)
			}
			if to == nil || !to.Equal(wantTo) {
				t.Fatalf("unexpected to %v", to)
			}
			if limit != 250 {
				t.Fatalf("unexpected limit %d", limit)
			}
			return wantSnapshots, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := httptest.NewRequest(
		http.MethodGet,
		"/v1/runs/run-1/resources?from="+wantFrom.Format(time.RFC3339)+"&to="+wantTo.Format(time.RFC3339)+"&limit=250",
		nil,
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", "run-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(context.WithValue(req.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	srv.handleListRunResources(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var got []domain.RunResourceSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "snap-1" {
		t.Fatalf("unexpected snapshots: %+v", got)
	}
}

func TestHandleListRunResources_RunNotFound(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, store.ErrRunNotFound
		},
		listRunResourceSnapshotsFn: func(context.Context, string, *time.Time, *time.Time, int) ([]domain.RunResourceSnapshot, error) {
			t.Fatal("ListRunResourceSnapshots should not be called when run is missing")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/run-1/resources", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", "run-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	w := httptest.NewRecorder()

	srv.handleListRunResources(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunResources_CrossProjectReturnsNotFound(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		getRunFn: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, ProjectID: "proj-2"}, nil
		},
		listRunResourceSnapshotsFn: func(context.Context, string, *time.Time, *time.Time, int) ([]domain.RunResourceSnapshot, error) {
			t.Fatal("ListRunResourceSnapshots should not be called for a different project")
			return nil, nil
		},
	}
	srv := newTestServer(t, ms, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/runs/run-1/resources", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("runID", "run-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(context.WithValue(req.Context(), ctxProjectIDKey, "proj-1"))
	w := httptest.NewRecorder()

	srv.handleListRunResources(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListRunResources_InvalidParams(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		query string
	}{
		{name: "invalid from", query: "?from=not-a-time"},
		{name: "invalid to", query: "?to=not-a-time"},
		{name: "invalid limit", query: "?limit=0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ms := &mockAPIStore{
				getRunFn: func(context.Context, string) (*domain.JobRun, error) {
					t.Fatal("GetRun should not be called for invalid query params")
					return nil, nil
				},
				listRunResourceSnapshotsFn: func(context.Context, string, *time.Time, *time.Time, int) ([]domain.RunResourceSnapshot, error) {
					t.Fatal("ListRunResourceSnapshots should not be called for invalid query params")
					return nil, nil
				},
			}
			srv := newTestServer(t, ms, nil, nil)

			req := httptest.NewRequest(http.MethodGet, "/v1/runs/run-1/resources"+tc.query, nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("runID", "run-1")
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			w := httptest.NewRecorder()

			srv.handleListRunResources(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}
