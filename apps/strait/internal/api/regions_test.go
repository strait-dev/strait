package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestLaunchInactiveRegionsEndpointNotRouted(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/regions", ""))

	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /v1/regions status = %d, want 404: %s", w.Code, w.Body.String())
	}
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

	if w.Code != http.StatusOK {
		t.Fatalf("GET project settings status = %d, want 200: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := body["default_region"]; ok {
		t.Fatalf("project settings exposed launch-inactive default_region: %#v", body)
	}
	if body["plan_tier"] != "starter" {
		t.Fatalf("plan_tier = %v, want starter", body["plan_tier"])
	}
	if body["max_key_lifetime_days"] != float64(30) {
		t.Fatalf("max_key_lifetime_days = %v, want 30", body["max_key_lifetime_days"])
	}
}

func TestHandleCreateJobDoesNotPersistOrReturnPreferredRegions(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
			if len(job.PreferredRegions) != 0 {
				t.Fatalf("CreateJob persisted launch-inactive preferred regions: %#v", job.PreferredRegions)
			}
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

	if w.Code != http.StatusCreated {
		t.Fatalf("create job status = %d, want 201: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := resp["preferred_regions"]; ok {
		t.Fatalf("create job response exposed launch-inactive preferred_regions: %#v", resp)
	}
}
