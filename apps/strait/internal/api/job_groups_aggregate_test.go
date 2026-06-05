package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/require"
)

func TestHandlePauseAllJobsByGroup_Success(t *testing.T) {
	t.Parallel()

	called := false
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "test-project"}, nil
		},
		PauseJobsByGroupFunc: func(_ context.Context, groupID string) error {
			called = true
			require.Equal(t, "group-1", groupID)

			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/group-1/pause-all", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, called)

}

func TestHandleResumeAllJobsByGroup_Success(t *testing.T) {
	t.Parallel()

	called := false
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "test-project"}, nil
		},
		ResumeJobsByGroupFunc: func(_ context.Context, groupID string) error {
			called = true
			require.Equal(t, "group-1", groupID)

			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/group-1/resume-all", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, called)

}

func TestHandleGetJobGroupStats_Success(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "test-project"}, nil
		},
		GetJobGroupStatsFunc: func(_ context.Context, groupID string) (*store.JobGroupStats, error) {
			return &store.JobGroupStats{GroupID: groupID, RunCounts: map[string]int{"completed": 4, "failed": 1}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/job-groups/group-1/stats", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

	var stats store.JobGroupStats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stats))
	require.EqualValues(t, 4, stats.RunCounts["completed"])

}
