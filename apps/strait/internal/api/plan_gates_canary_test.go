package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"
)

func TestCanaryDeploymentUpdate_FreeTierRejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
			return &domain.Workflow{ID: id, ProjectID: "proj-1", Name: "Workflow", Slug: "workflow", Version: 1}, nil
		},
		UpdateCanaryDeploymentTrafficFunc: func(context.Context, string, int) error {
			t.Fatal("UpdateCanaryDeploymentTraffic must not be called when canary gate rejects")
			return nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanFree)})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPatch, "/v1/workflows/wf-1/canary", `{"traffic_pct":50}`, "proj-1"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("free-tier canary update must be 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Canary deployments") {
		t.Fatalf("rejection must name the feature, got: %s", w.Body.String())
	}
}

func TestDeploymentVersionCanaryStrategy_FreeTierRejected(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(context.Context, *domain.DeploymentVersion) error {
			t.Fatal("CreateDeploymentVersion must not be called when canary gate rejects")
			return nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanFree)})

	body := `{
		"project_id":"proj-1",
		"environment":"production",
		"runtime":"node",
		"artifact_uri":"https://example.com/artifacts/dep-canary.tgz",
		"strategy":"canary",
		"canary_percent":25
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/deployments", body, "proj-1"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("free-tier canary deployment strategy must be 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Canary deployments") {
		t.Fatalf("rejection must name the feature, got: %s", w.Body.String())
	}
}

func TestDeploymentVersionCanaryStrategy_ScaleTierAllowed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		CreateDeploymentVersionFunc: func(_ context.Context, deployment *domain.DeploymentVersion) error {
			deployment.ID = "dep-canary"
			deployment.CreatedAt = time.Now()
			deployment.UpdatedAt = deployment.CreatedAt
			return nil
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, &tunableLimitsEnforcer{limits: billing.GetPlanLimits(domain.PlanScale)})

	body := `{
		"project_id":"proj-1",
		"environment":"production",
		"runtime":"node",
		"artifact_uri":"https://example.com/artifacts/dep-canary.tgz",
		"strategy":"canary",
		"canary_percent":25
	}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedProjectRequest(http.MethodPost, "/v1/deployments", body, "proj-1"))

	if w.Code != http.StatusCreated {
		t.Fatalf("scale-tier canary deployment strategy must pass, got %d: %s", w.Code, w.Body.String())
	}
}
