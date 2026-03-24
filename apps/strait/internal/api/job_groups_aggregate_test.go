package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/store"
)

func TestHandlePauseAllJobsByGroup_Success(t *testing.T) {
	t.Parallel()

	called := false
	ms := &APIStoreMock{
		PauseJobsByGroupFunc: func(_ context.Context, groupID string) error {
			called = true
			if groupID != "group-1" {
				t.Fatalf("groupID = %q, want %q", groupID, "group-1")
			}
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/group-1/pause-all", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("PauseJobsByGroup was not called")
	}
}

func TestHandleResumeAllJobsByGroup_Success(t *testing.T) {
	t.Parallel()

	called := false
	ms := &APIStoreMock{
		ResumeJobsByGroupFunc: func(_ context.Context, groupID string) error {
			called = true
			if groupID != "group-1" {
				t.Fatalf("groupID = %q, want %q", groupID, "group-1")
			}
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/group-1/resume-all", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !called {
		t.Fatal("ResumeJobsByGroup was not called")
	}
}

func TestHandleGetJobGroupStats_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobGroupStatsFunc: func(_ context.Context, groupID string) (*store.JobGroupStats, error) {
			return &store.JobGroupStats{GroupID: groupID, RunCounts: map[string]int{"completed": 4, "failed": 1}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/job-groups/group-1/stats", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stats store.JobGroupStats
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if stats.RunCounts["completed"] != 4 {
		t.Fatalf("completed count = %d, want 4", stats.RunCounts["completed"])
	}
}
