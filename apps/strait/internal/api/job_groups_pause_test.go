package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

func TestPauseAllJobsByGroup_Success(t *testing.T) {
	t.Parallel()
	var pauseCalled bool
	ms := &APIStoreMock{
		PauseJobsByGroupFunc: func(_ context.Context, groupID string) error {
			if groupID != "group-1" {
				t.Fatalf("expected groupID group-1, got %s", groupID)
			}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !pauseCalled {
		t.Fatal("expected PauseJobsByGroup to be called")
	}
}

func TestResumeAllJobsByGroup_Success(t *testing.T) {
	t.Parallel()
	var resumeCalled bool
	ms := &APIStoreMock{
		ResumeJobsByGroupFunc: func(_ context.Context, groupID string) error {
			if groupID != "group-1" {
				t.Fatalf("expected groupID group-1, got %s", groupID)
			}
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

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !resumeCalled {
		t.Fatal("expected ResumeJobsByGroup to be called")
	}
}
