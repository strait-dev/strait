package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"context"

	"strait/internal/domain"
)

func TestListOrgRuns_ReturnsRunsFromAllProjects(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, orgID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			if orgID != "org-1" {
				t.Fatalf("expected org-1, got %q", orgID)
			}
			return []domain.JobRun{
				{ID: "run-1", ProjectID: "proj-1", JobID: "job-1", CreatedAt: now},
				{ID: "run-2", ProjectID: "proj-2", JobID: "job-2", CreatedAt: now.Add(-time.Second)},
				{ID: "run-3", ProjectID: "proj-1", JobID: "job-3", CreatedAt: now.Add(-2 * time.Second)},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/org-1/runs", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}

	// Verify runs come from different projects.
	projectIDs := map[string]bool{}
	for _, r := range runs {
		projectIDs[r.ProjectID] = true
	}
	if len(projectIDs) != 2 {
		t.Fatalf("expected runs from 2 different projects, got %d", len(projectIDs))
	}
}

func TestListOrgRuns_CrossOrg_Forbidden(t *testing.T) {
	t.Parallel()
	rawKey := "strait_" + strings.Repeat("ee", 32)

	ms := &APIStoreMock{
		GetAPIKeyByHashFunc: func(_ context.Context, _ string) (*domain.APIKey, error) {
			return &domain.APIKey{ID: "key-1", ProjectID: "proj-1", OrgID: "org-1"}, nil
		},
		TouchAPIKeyLastUsedFunc: func(_ context.Context, _ string) error { return nil },
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/org-different/runs", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}

	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Error == nil || resp.Error.Message != "api key does not belong to this organization" {
		t.Fatalf("expected org mismatch error, got %+v", resp.Error)
	}
}

func TestListOrgRuns_RequiresAuth(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/org-1/runs", nil)

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListOrgJobs_ReturnsJobsFromAllProjects(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()

	ms := &APIStoreMock{
		ListJobsByOrgFunc: func(_ context.Context, orgID string, _ int, _ *time.Time) ([]domain.Job, error) {
			if orgID != "org-1" {
				t.Fatalf("expected org-1, got %q", orgID)
			}
			return []domain.Job{
				{ID: "job-1", ProjectID: "proj-1", Name: "Job One", CreatedAt: now},
				{ID: "job-2", ProjectID: "proj-2", Name: "Job Two", CreatedAt: now.Add(-time.Second)},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/org-1/jobs", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var jobs []domain.Job
	decodePaginatedList(t, w.Body.Bytes(), &jobs)
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].ProjectID != "proj-1" || jobs[1].ProjectID != "proj-2" {
		t.Fatalf("expected jobs from proj-1 and proj-2, got %q and %q", jobs[0].ProjectID, jobs[1].ProjectID)
	}
}

func TestListOrgRuns_Pagination_Works(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	callCount := 0

	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, _ string, limit int, cursor *time.Time) ([]domain.JobRun, error) {
			callCount++
			// First call: return limit+1 items to indicate has_more.
			if cursor == nil {
				runs := make([]domain.JobRun, 0, limit)
				for i := range limit {
					runs = append(runs, domain.JobRun{
						ID:        "run-" + string(rune('A'+i)),
						ProjectID: "proj-1",
						CreatedAt: now.Add(-time.Duration(i) * time.Second),
					})
				}
				return runs, nil
			}
			return nil, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/org-1/runs?limit=2", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var envelope struct {
		Data       json.RawMessage `json:"data"`
		HasMore    bool            `json:"has_more"`
		NextCursor *string         `json:"next_cursor,omitempty"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	var runs []domain.JobRun
	if err := json.Unmarshal(envelope.Data, &runs); err != nil {
		t.Fatalf("invalid data JSON: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
}

func TestListOrgRuns_EmptyOrg_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/org-empty/runs", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs, got %d", len(runs))
	}
}
