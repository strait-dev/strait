package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/store"
)

func TestHandleGetRegions(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/regions", ""))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Regions []RegionResponse `json:"regions"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Regions) == 0 {
		t.Fatal("expected at least one region")
	}
	if !sort.SliceIsSorted(resp.Regions, func(i, j int) bool {
		return resp.Regions[i].Code < resp.Regions[j].Code
	}) {
		t.Fatalf("expected regions sorted by code, got %#v", resp.Regions)
	}

	for _, region := range resp.Regions {
		if region.Code == "" || region.Label == "" || region.City == "" || region.Country == "" || region.Continent == "" {
			t.Fatalf("expected complete region metadata, got %#v", region)
		}
		if len(region.Availability) != len(domain.AllPlanTiers()) {
			t.Fatalf("expected availability for every plan tier, got %#v", region)
		}
	}
}

func TestHandleGetProjectSettings(t *testing.T) {
	t.Parallel()

	t.Run("with_default_region", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
				return &store.ProjectQuota{
					ProjectID:     projectID,
					DefaultRegion: "lhr",
					PlanTier:      "starter",
				}, nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/proj-1/settings/", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp ProjectSettingsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if resp.DefaultRegion != "lhr" {
			t.Errorf("expected default_region=lhr, got %q", resp.DefaultRegion)
		}
		if resp.PlanTier != "starter" {
			t.Errorf("expected plan_tier=starter, got %q", resp.PlanTier)
		}
	})

	t.Run("no_quota_row", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
				return nil, nil // no row
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodGet, "/v1/projects/proj-1/settings/", ""))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var resp ProjectSettingsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if resp.DefaultRegion != "" {
			t.Errorf("expected empty default_region, got %q", resp.DefaultRegion)
		}
		if resp.PlanTier != "free" {
			t.Errorf("expected plan_tier=free, got %q", resp.PlanTier)
		}
	})
}

func TestHandleUpdateProjectSettings(t *testing.T) {
	t.Parallel()

	t.Run("set_valid_region", func(t *testing.T) {
		t.Parallel()
		var updatedRegion string
		ms := &APIStoreMock{
			UpdateProjectDefaultRegionFunc: func(_ context.Context, projectID, region string) error {
				updatedRegion = region
				return nil
			},
			GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
				return &store.ProjectQuota{
					ProjectID:     projectID,
					DefaultRegion: updatedRegion,
				}, nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPut, "/v1/projects/proj-1/settings/", `{"default_region":"nrt"}`))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if updatedRegion != "nrt" {
			t.Errorf("expected region update to nrt, got %q", updatedRegion)
		}
	})

	t.Run("clear_region", func(t *testing.T) {
		t.Parallel()
		var updatedRegion string
		ms := &APIStoreMock{
			UpdateProjectDefaultRegionFunc: func(_ context.Context, projectID, region string) error {
				updatedRegion = region
				return nil
			},
			GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
				return &store.ProjectQuota{
					ProjectID:     projectID,
					DefaultRegion: updatedRegion,
				}, nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPut, "/v1/projects/proj-1/settings/", `{"default_region":""}`))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if updatedRegion != "" {
			t.Errorf("expected empty region, got %q", updatedRegion)
		}
	})

	t.Run("invalid_region", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{}
		srv := newTestServer(t, ms, &mockQueue{}, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPut, "/v1/projects/proj-1/settings/", `{"default_region":"invalid"}`))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleCreateJob_InvalidRegion(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

	body := `{
		"project_id": "proj-1",
		"name": "Test Job",
		"slug": "test-job",
		"endpoint_url": "https://example.com/callback",
		"preferred_regions": ["invalid-region"]
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateJob_ValidRegion(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		CreateJobFunc: func(_ context.Context, job *domain.Job) error {
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
		"preferred_regions": ["lhr"]
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCreateJob_RegionGating(t *testing.T) {
	t.Parallel()

	t.Run("free_plan_disallowed_region", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
				return &store.ProjectQuota{
					ProjectID: projectID,
					PlanTier:  "free",
				}, nil
			},
		}
		cfg := &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
			EnforceRegionGating: true,
		}
		srv := NewServer(ServerDeps{Config: cfg, Store: ms, Queue: &mockQueue{}})
		t.Cleanup(srv.Close)

		body := `{
			"project_id": "proj-1",
			"name": "Test Job",
			"slug": "test-job",
			"endpoint_url": "https://example.com/callback",
			"preferred_regions": ["lhr"]
		}`

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("starter_plan_preferred_regions_allowed_by_catalog", func(t *testing.T) {
		t.Parallel()
		// Region gating is now driven by the generated billing catalog. Starter
		// has no allowed-region list there, so any valid region is allowed.
		ms := &APIStoreMock{
			GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
				return &store.ProjectQuota{
					ProjectID: projectID,
					PlanTier:  "starter",
				}, nil
			},
		}
		cfg := &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
			EnforceRegionGating: true,
		}
		srv := NewServer(ServerDeps{Config: cfg, Store: ms, Queue: &mockQueue{}})
		t.Cleanup(srv.Close)

		body := `{
			"project_id": "proj-1",
			"name": "Test Job",
			"slug": "test-job",
			"endpoint_url": "https://example.com/callback",
			"preferred_regions": ["lhr"]
		}`

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 for starter plan with preferred_regions, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("gating_disabled_allows_all", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			CreateJobFunc: func(_ context.Context, job *domain.Job) error {
				job.ID = "job-123"
				return nil
			},
		}
		// No EnforceRegionGating set (default false).
		srv := newTestServer(t, ms, &mockQueue{}, nil)

		body := `{
			"project_id": "proj-1",
			"name": "Test Job",
			"slug": "test-job",
			"endpoint_url": "https://example.com/callback",
			"preferred_regions": ["hkg"]
		}`

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 (gating disabled), got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleCreateJob_PreferredRegions(t *testing.T) {
	t.Parallel()

	t.Run("valid_preferred_regions", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			CreateJobFunc: func(_ context.Context, job *domain.Job) error {
				job.ID = "job-123"
				if len(job.PreferredRegions) != 2 {
					t.Errorf("expected 2 preferred regions, got %d", len(job.PreferredRegions))
				}
				if job.PreferredRegions[0] != "iad" || job.PreferredRegions[1] != "lhr" {
					t.Errorf("expected [iad, lhr], got %v", job.PreferredRegions)
				}
				return nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)

		body := `{
			"project_id": "proj-1",
			"name": "Multi Region Job",
			"slug": "multi-region-job",
			"endpoint_url": "https://example.com/callback",
			"preferred_regions": ["iad", "lhr"]
		}`

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid_preferred_region", func(t *testing.T) {
		t.Parallel()
		srv := newTestServer(t, &APIStoreMock{}, &mockQueue{}, nil)

		body := `{
			"project_id": "proj-1",
			"name": "Bad Region Job",
			"slug": "bad-region-job",
			"endpoint_url": "https://example.com/callback",
			"preferred_regions": ["iad", "invalid"]
		}`

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("multi_region_gated_on_free_plan", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetProjectQuotaFunc: func(_ context.Context, projectID string) (*store.ProjectQuota, error) {
				return &store.ProjectQuota{
					ProjectID: projectID,
					PlanTier:  "free",
				}, nil
			},
		}
		cfg := &config.Config{
			InternalSecret:      "test-secret-value",
			MaxBulkTriggerItems: 500,
			JWTSigningKey:       testJWTSigningKey,
			EnforceRegionGating: true,
		}
		srv := NewServer(ServerDeps{Config: cfg, Store: ms, Queue: &mockQueue{}})
		t.Cleanup(srv.Close)

		body := `{
			"project_id": "proj-1",
			"name": "Multi Region Job",
			"slug": "multi-region-job",
			"endpoint_url": "https://example.com/callback",
			"preferred_regions": ["iad", "lhr"]
		}`

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/", body))

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleUpdateJob_PreferredRegions(t *testing.T) {
	t.Parallel()

	t.Run("update_preferred_regions", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
				return &domain.Job{
					ID:          id,
					ProjectID:   "proj-1",
					Name:        "Test",
					Slug:        "test",
					EndpointURL: "https://example.com/callback",
				}, nil
			},
			UpdateJobFunc: func(_ context.Context, job *domain.Job) error {
				if len(job.PreferredRegions) != 3 {
					t.Errorf("expected 3 preferred regions, got %d", len(job.PreferredRegions))
				}
				return nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123/", `{"preferred_regions":["iad","lhr","nrt"]}`))

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid_preferred_region_on_update", func(t *testing.T) {
		t.Parallel()
		ms := &APIStoreMock{
			GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
				return &domain.Job{
					ID:          id,
					ProjectID:   "proj-1",
					Name:        "Test",
					Slug:        "test",
					EndpointURL: "https://example.com/callback",
				}, nil
			},
		}
		srv := newTestServer(t, ms, &mockQueue{}, nil)

		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123/", `{"preferred_regions":["iad","invalid"]}`))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestHandleUpdateJob_InvalidRegion(t *testing.T) {
	t.Parallel()
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return &domain.Job{
				ID:          id,
				ProjectID:   "proj-1",
				Name:        "Test",
				Slug:        "test",
				EndpointURL: "https://example.com/callback",
			}, nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPatch, "/v1/jobs/job-123/", `{"preferred_regions":["invalid-region"]}`))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
