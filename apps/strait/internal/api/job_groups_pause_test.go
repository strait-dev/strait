package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestPauseAllJobsByGroup_Success(t *testing.T) {
	t.Parallel()
	var pauseCalled bool
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "test-project"}, nil
		},
		PauseJobsByGroupFunc: func(_ context.Context, groupID string) error {
			require.Equal(t, "group-1",
				groupID)

			pauseCalled = true
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/group-1/pause-all", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, pauseCalled,
	)
}

func TestResumeAllJobsByGroup_Success(t *testing.T) {
	t.Parallel()
	var resumeCalled bool
	ms := &APIStoreMock{
		GetJobGroupFunc: func(_ context.Context, id string) (*domain.JobGroup, error) {
			return &domain.JobGroup{ID: id, ProjectID: "test-project"}, nil
		},
		ResumeJobsByGroupFunc: func(_ context.Context, groupID string) error {
			require.Equal(t, "group-1",
				groupID)

			resumeCalled = true
			return nil
		},
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/job-groups/group-1/resume-all", ""))
	require.Equal(t, http.StatusOK,
		w.Code)
	require.True(
		t, resumeCalled,
	)
}
