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

	"github.com/stretchr/testify/require"
)

func TestListOrgRuns_ReturnsRunsFromAllProjects(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	const orgUUID = "00000000-0000-4000-8000-000000000001"

	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, orgID string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			require.Equal(t, orgUUID,
				orgID,
			)

			return []domain.JobRun{
				{ID: "run-1", ProjectID: "proj-1", JobID: "job-1", CreatedAt: now},
				{ID: "run-2", ProjectID: "proj-2", JobID: "job-2", CreatedAt: now.Add(-time.Second)},
				{ID: "run-3", ProjectID: "proj-1", JobID: "job-3", CreatedAt: now.Add(-2 * time.Second)},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/"+orgUUID+"/runs", ""))
	require.Equal(t, http.
		StatusOK,
		w.Code)

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	require.Len(t, runs,
		3)

	// Verify runs come from different projects.
	projectIDs := map[string]bool{}
	for _, r := range runs {
		projectIDs[r.ProjectID] = true
	}
	require.Len(t, projectIDs,
		2)
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
	require.Equal(t, http.
		StatusForbidden,
		w.Code)

	var resp ErrorResponse
	require.NoError(t,
		json.Unmarshal(w.Body.Bytes(), &resp))
	require.False(t, resp.
		Error ==
		nil || resp.Error.Message !=
		"api key does not belong to this organization",
	)
}

func TestListOrgRuns_RequiresAuth(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/organizations/org-1/runs", nil)

	srv.ServeHTTP(w, req)
	require.Equal(t, http.
		StatusUnauthorized,
		w.Code)
}

func TestListOrgJobs_ReturnsJobsFromAllProjects(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	const orgUUID = "00000000-0000-4000-8000-000000000001"

	ms := &APIStoreMock{
		ListJobsByOrgFunc: func(_ context.Context, orgID string, _ int, _ *time.Time) ([]domain.Job, error) {
			require.Equal(t, orgUUID,
				orgID,
			)

			return []domain.Job{
				{ID: "job-1", ProjectID: "proj-1", Name: "Job One", CreatedAt: now},
				{ID: "job-2", ProjectID: "proj-2", Name: "Job Two", CreatedAt: now.Add(-time.Second)},
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/"+orgUUID+"/jobs", ""))
	require.Equal(t, http.
		StatusOK,
		w.Code)

	var jobs []domain.Job
	decodePaginatedList(t, w.Body.Bytes(), &jobs)
	require.Len(t, jobs,
		2)
	require.False(t, jobs[0].ProjectID !=
		"proj-1" || jobs[1].ProjectID !=
		"proj-2",
	)
}

func TestListOrgRuns_Pagination_Works(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	callCount := 0
	const orgUUID = "00000000-0000-4000-8000-000000000001"

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

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/"+orgUUID+"/runs?limit=2", ""))
	require.Equal(t, http.
		StatusOK,
		w.Code)

	var envelope struct {
		Data       json.RawMessage `json:"data"`
		HasMore    bool            `json:"has_more"`
		NextCursor *string         `json:"next_cursor,omitempty"`
	}
	require.NoError(t,
		json.Unmarshal(w.Body.Bytes(), &envelope))

	var runs []domain.JobRun
	require.NoError(t,
		json.Unmarshal(envelope.Data, &runs))
	require.Len(t, runs,
		2)
}

func TestListOrgRuns_EmptyOrg_ReturnsEmpty(t *testing.T) {
	t.Parallel()
	const orgUUID = "00000000-0000-4000-8000-0000000000ee"

	ms := &APIStoreMock{
		ListRunsByOrgFunc: func(_ context.Context, _ string, _ int, _ *time.Time) ([]domain.JobRun, error) {
			return []domain.JobRun{}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/organizations/"+orgUUID+"/runs", ""))
	require.Equal(t, http.
		StatusOK,
		w.Code)

	var runs []domain.JobRun
	decodePaginatedList(t, w.Body.Bytes(), &runs)
	require.Empty(t, runs)
}
