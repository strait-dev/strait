package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestCreateDeploymentVersion(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		createDeploymentVersionFn: func(_ context.Context, deployment *domain.DeploymentVersion) error {
			deployment.ID = "dep-1"
			deployment.CreatedAt = time.Now()
			deployment.UpdatedAt = deployment.CreatedAt
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{
		"project_id":"proj-1",
		"environment":"production",
		"runtime":"node",
		"artifact_uri":"https://example.com/artifacts/dep-1.tgz",
		"manifest":{"jobs":1},
		"checksum":"sha256:abc"
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response["id"] != "dep-1" {
		t.Fatalf("id = %v, want dep-1", response["id"])
	}
	if response["status"] != string(domain.DeploymentVersionStatusDraft) {
		t.Fatalf("status = %v, want draft", response["status"])
	}
}

func TestCreateDeploymentVersion_WithCanaryStrategy(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		createDeploymentVersionFn: func(_ context.Context, deployment *domain.DeploymentVersion) error {
			deployment.ID = "dep-canary"
			deployment.CreatedAt = time.Now()
			deployment.UpdatedAt = deployment.CreatedAt
			if deployment.Strategy != domain.DeploymentStrategyCanary {
				return errors.New("expected canary strategy")
			}
			if deployment.CanaryPercent == nil || *deployment.CanaryPercent != 25 {
				return errors.New("expected canary_percent = 25")
			}
			if deployment.CanaryDuration == nil || *deployment.CanaryDuration != 10*time.Minute {
				return errors.New("expected canary_duration = 10m")
			}
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{
		"project_id":"proj-1",
		"environment":"production",
		"runtime":"node",
		"artifact_uri":"https://example.com/artifacts/dep-canary.tgz",
		"strategy":"canary",
		"canary_percent":25,
		"canary_duration":"10m"
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response["strategy"] != "canary" {
		t.Fatalf("strategy = %v, want canary", response["strategy"])
	}
}

func TestCreateDeploymentVersion_CanaryRequiresPercent(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id":"proj-1",
		"environment":"production",
		"runtime":"node",
		"artifact_uri":"https://example.com/artifacts/dep-1.tgz",
		"strategy":"canary"
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateDeploymentVersion_DirectDefault(t *testing.T) {
	t.Parallel()

	var captured *domain.DeploymentVersion
	ms := &mockAPIStore{
		createDeploymentVersionFn: func(_ context.Context, deployment *domain.DeploymentVersion) error {
			captured = deployment
			deployment.ID = "dep-direct"
			deployment.CreatedAt = time.Now()
			deployment.UpdatedAt = deployment.CreatedAt
			return nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	body := `{
		"project_id":"proj-1",
		"environment":"production",
		"runtime":"node",
		"artifact_uri":"https://example.com/artifacts/dep-1.tgz"
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if captured.Strategy != domain.DeploymentStrategyDirect {
		t.Fatalf("strategy = %v, want direct", captured.Strategy)
	}

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response["strategy"] != "direct" {
		t.Fatalf("strategy = %v, want direct", response["strategy"])
	}
}

func TestCreateDeploymentVersion_InvalidStrategy(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	body := `{
		"project_id":"proj-1",
		"environment":"production",
		"runtime":"node",
		"artifact_uri":"https://example.com/artifacts/dep-1.tgz",
		"strategy":"blue_green"
	}`

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestFinalizeDeploymentVersion_NotFound(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		finalizeDeploymentVersionFn: func(_ context.Context, _, _, _ string) (*domain.DeploymentVersion, error) {
			return nil, store.ErrDeploymentVersionNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-404/finalize", `{"project_id":"proj-1","environment":"production"}`))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPromoteDeploymentVersion(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		promoteDeploymentVersionFn: func(_ context.Context, deploymentID, projectID, environment, _ string) (*domain.DeploymentVersion, error) {
			if deploymentID != "dep-2" || projectID != "proj-1" || environment != "production" {
				return nil, errors.New("unexpected promote input")
			}
			now := time.Now()
			return &domain.DeploymentVersion{
				ID:          deploymentID,
				ProjectID:   projectID,
				Environment: environment,
				Status:      domain.DeploymentVersionStatusPromoted,
				PromotedAt:  &now,
			}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-2/promote", `{"project_id":"proj-1","environment":"production"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response["status"] != string(domain.DeploymentVersionStatusPromoted) {
		t.Fatalf("status = %v, want promoted", response["status"])
	}
}

func TestListDeploymentVersions(t *testing.T) {
	t.Parallel()

	ms := &mockAPIStore{
		listDeploymentVersionsFn: func(_ context.Context, projectID, environment string, limit int, _ *time.Time) ([]domain.DeploymentVersion, error) {
			if projectID != "proj-1" || environment != "production" {
				return nil, errors.New("unexpected filters")
			}
			return []domain.DeploymentVersion{{ID: "dep-1", ProjectID: projectID, Environment: environment, Status: domain.DeploymentVersionStatusDraft, CreatedAt: time.Now()}}, nil
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodGet, "/v1/deployments?environment=production", "", "proj-1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var response []map[string]any
	decodePaginatedList(t, w.Body.Bytes(), &response)
	if len(response) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(response))
	}
	if response[0]["id"] != "dep-1" {
		t.Fatalf("id = %v, want dep-1", response[0]["id"])
	}
}
