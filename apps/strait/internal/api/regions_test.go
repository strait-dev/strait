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

func TestLaunchInactiveRegionsEndpointNotRouted(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/regions", ""))
	require.Equal(t, http.StatusNotFound,
		w.Code,
	)
}

func TestHandleGetProjectSettingsDoesNotExposeDefaultRegion(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
			return &store.ProjectQuota{
				ProjectID:          projectID,
				DefaultRegion:      "lhr",
				PlanTier:           "starter",
				MaxKeyLifetimeDays: 30,
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/proj-1/settings/", ""))
	require.Equal(t, http.StatusOK,
		w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	if _, ok := body["default_region"]; ok {
		require.Failf(t, "test failure",

			"project settings exposed launch-inactive default_region: %#v", body)
	}
	require.Equal(t, "starter", body["plan_tier"])
	require.InDelta(t, float64(30),
		body["max_key_lifetime_days"], 1e-9)
}

func TestHandleCreateJobDoesNotPersistOrReturnPreferredRegions(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			require.Empty(t,
				job.PreferredRegions)

			job.ID = "job-123"
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "Test Job",
		"slug": "test-job",
		"endpoint_url": "https://example.com/callback",
		"preferred_regions": ["iad", "lhr"]
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	if _, ok := resp["preferred_regions"]; ok {
		require.Failf(t, "test failure",

			"create job response exposed launch-inactive preferred_regions: %#v", resp)
	}
}
