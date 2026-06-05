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

	"github.com/stretchr/testify/require"
)

func TestCreateDeploymentVersion(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(_ context.Context, deployment *domain.DeploymentVersion) error {
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &response,
	))
	require.Equal(t, "dep-1", response["id"])
	require.Equal(t, string(domain.
		DeploymentVersionStatusDraft,
	), response["status"])

}

func TestCreateDeploymentVersion_WithCanaryStrategy(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(_ context.Context, deployment *domain.DeploymentVersion) error {
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &response,
	))
	require.Equal(t, "canary", response["strategy"])

}

func TestCreateDeploymentVersion_CanaryRequiresPercent(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestCreateDeploymentVersion_DirectDefault(t *testing.T) {
	t.Parallel()

	var captured *domain.DeploymentVersion
	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(_ context.Context, deployment *domain.DeploymentVersion) error {
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
	require.Equal(t, http.StatusCreated,
		w.Code,
	)
	require.Equal(t, domain.DeploymentStrategyDirect,

		captured.
			Strategy)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &response,
	))
	require.Equal(t, "direct", response["strategy"])

}

func TestCreateDeploymentVersion_InvalidStrategy(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{}
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
	require.Equal(t, http.StatusBadRequest,

		w.Code)

}

func TestFinalizeDeploymentVersion_NotFound(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		FinalizeDeploymentVersionFunc: func(_ context.Context, _, _, _ string) (*domain.DeploymentVersion, error) {
			return nil, store.ErrDeploymentVersionNotFound
		},
	}

	srv := newTestServer(t, ms, &mockQueue{}, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/deployments/dep-404/finalize", `{"project_id":"proj-1","environment":"production"}`))
	require.Equal(t, http.StatusNotFound,
		w.
			Code)

}

func TestPromoteDeploymentVersion(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		PromoteDeploymentVersionFunc: func(_ context.Context, deploymentID, projectID, environment, _ string) (*domain.DeploymentVersion, error) {
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
	require.Equal(t, http.StatusOK,
		w.Code)

	var response map[string]any
	require.NoError(t, json.Unmarshal(w.Body.
		Bytes(), &response,
	))
	require.Equal(t, string(domain.
		DeploymentVersionStatusPromoted,
	), response["status"])

}
