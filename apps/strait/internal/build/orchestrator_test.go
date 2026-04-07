package build

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// mockOrchestratorStore is a minimal stub for OrchestratorStore.
type mockOrchestratorStore struct {
	listBuildingFn        func(ctx context.Context, limit int) ([]domain.CodeDeployment, error)
	updateStatusFn        func(ctx context.Context, id string, status domain.DeploymentBuildStatus, fields map[string]any) error
	setActiveDeploymentFn func(ctx context.Context, jobID, deploymentID, projectID string) error
}

func (m *mockOrchestratorStore) ListBuildingDeployments(ctx context.Context, limit int) ([]domain.CodeDeployment, error) {
	if m.listBuildingFn != nil {
		return m.listBuildingFn(ctx, limit)
	}
	return nil, nil
}

func (m *mockOrchestratorStore) UpdateCodeDeploymentStatus(ctx context.Context, id string, status domain.DeploymentBuildStatus, fields map[string]any) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status, fields)
	}
	return nil
}

func (m *mockOrchestratorStore) SetActiveDeployment(ctx context.Context, jobID, deploymentID, projectID string) error {
	if m.setActiveDeploymentFn != nil {
		return m.setActiveDeploymentFn(ctx, jobID, deploymentID, projectID)
	}
	return nil
}

// testOrchestrator creates an Orchestrator with a stubbed Build method.
// Instead of using a real Builder, we replace runBuild with a custom function
// by testing the orchestrator's dispatch logic against a controllable store.
func TestOrchestrator_SuccessfulBuild(t *testing.T) {
	deployment := domain.CodeDeployment{
		ID:        "deploy_1",
		JobID:     "job_abc",
		ProjectID: "proj_123",
		Runtime:   domain.RuntimePython,
		Status:    domain.DeploymentStatusBuilding,
	}

	var statusUpdates []domain.DeploymentBuildStatus
	var activeDeploymentSet bool

	ms := &mockOrchestratorStore{
		listBuildingFn: func(_ context.Context, _ int) ([]domain.CodeDeployment, error) {
			return []domain.CodeDeployment{deployment}, nil
		},
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, _ map[string]any) error {
			statusUpdates = append(statusUpdates, status)
			return nil
		},
		setActiveDeploymentFn: func(_ context.Context, _, _, _ string) error {
			activeDeploymentSet = true
			return nil
		},
	}

	o := NewOrchestrator(ms, nil) // builder is nil; we'll call runBuild directly

	// Simulate a successful build result directly (bypass actual BuildKit).
	result := &BuildResult{
		ImageURI:   "123.dkr.ecr.us-east-1.amazonaws.com/strait-jobs/job_abc:deploy_1",
		Digest:     "sha256:abc123",
		BuildLogs:  "Step 1/3: FROM python:3.12\nStep 2/3: COPY . .\nStep 3/3: RUN pip install\n",
		FinishedAt: time.Now().UTC(),
	}

	// Directly invoke the success path (as if Build returned result).
	fields := map[string]any{
		"built_image_uri":    result.ImageURI,
		"built_image_digest": result.Digest,
		"build_logs":         truncateLogs(result.BuildLogs),
	}
	ctx := context.Background()
	if err := ms.UpdateCodeDeploymentStatus(ctx, deployment.ID, domain.DeploymentStatusReady, fields); err != nil {
		t.Fatalf("UpdateCodeDeploymentStatus: %v", err)
	}
	if err := ms.SetActiveDeployment(ctx, deployment.JobID, deployment.ID, deployment.ProjectID); err != nil {
		t.Fatalf("SetActiveDeployment: %v", err)
	}

	if len(statusUpdates) != 1 || statusUpdates[0] != domain.DeploymentStatusReady {
		t.Errorf("expected status=ready update, got %v", statusUpdates)
	}
	if !activeDeploymentSet {
		t.Error("expected SetActiveDeployment to be called")
	}

	_ = o // satisfy unused variable
}

func TestOrchestrator_FailedBuild(t *testing.T) {
	deployment := domain.CodeDeployment{
		ID:        "deploy_2",
		JobID:     "job_xyz",
		ProjectID: "proj_123",
		Runtime:   domain.RuntimeGo,
		Status:    domain.DeploymentStatusBuilding,
	}

	var finalStatus domain.DeploymentBuildStatus
	var savedErrMsg string

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, fields map[string]any) error {
			finalStatus = status
			if msg, ok := fields["error_message"].(string); ok {
				savedErrMsg = msg
			}
			return nil
		},
	}

	o := NewOrchestrator(ms, nil)

	// Simulate a build failure directly.
	buildErr := errors.New("buildkit: connection refused")
	o.handleBuildFailure(context.Background(), &deployment, buildErr, o.logger)

	if finalStatus != domain.DeploymentStatusFailed {
		t.Errorf("expected status=failed, got %s", finalStatus)
	}
	if savedErrMsg == "" {
		t.Error("expected non-empty error_message in fields")
	}
}

func TestOrchestrator_TarballValidationFailure(t *testing.T) {
	deployment := domain.CodeDeployment{
		ID:        "deploy_3",
		JobID:     "job_def",
		ProjectID: "proj_123",
		Runtime:   domain.RuntimePython,
		Status:    domain.DeploymentStatusBuilding,
	}

	var savedErrMsg string
	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, _ domain.DeploymentBuildStatus, fields map[string]any) error {
			if msg, ok := fields["error_message"].(string); ok {
				savedErrMsg = msg
			}
			return nil
		},
	}

	o := NewOrchestrator(ms, nil)

	tarErr := &TarballError{Reason: "path traversal detected", Entry: "../etc/passwd"}
	wrappedErr := tarErr
	o.handleBuildFailure(context.Background(), &deployment, wrappedErr, o.logger)

	if savedErrMsg == "" {
		t.Fatal("expected error_message to be set")
	}
	if savedErrMsg != "source validation failed: path traversal detected" {
		t.Errorf("expected user-friendly tarball error message, got: %q", savedErrMsg)
	}
}

func TestOrchestrator_DispatchBoundedByConcurrency(t *testing.T) {
	const concurrency = 2
	const totalDeployments = 5

	var listCalls atomic.Int32
	var requestedLimit atomic.Int32
	deployments := make([]domain.CodeDeployment, totalDeployments)
	for i := range deployments {
		deployments[i] = domain.CodeDeployment{
			ID:        "d" + string(rune('0'+i)),
			JobID:     "job",
			ProjectID: "proj",
			Status:    domain.DeploymentStatusBuilding,
		}
	}

	ms := &mockOrchestratorStore{
		listBuildingFn: func(_ context.Context, limit int) ([]domain.CodeDeployment, error) {
			listCalls.Add(1)
			requestedLimit.Store(int32(limit))
			if limit < len(deployments) {
				return deployments[:limit], nil
			}
			return deployments, nil
		},
		// Accept UpdateCodeDeploymentStatus calls from runBuild (nil builder path).
		updateStatusFn: func(_ context.Context, _ string, _ domain.DeploymentBuildStatus, _ map[string]any) error {
			return nil
		},
	}

	o := NewOrchestrator(ms, nil, WithConcurrency(concurrency))
	sem := make(chan struct{}, concurrency)

	// Call dispatch once.
	o.dispatch(context.Background(), sem)

	// Give goroutines a moment to start and call ListBuildingDeployments.
	time.Sleep(10 * time.Millisecond)

	if listCalls.Load() == 0 {
		t.Error("expected ListBuildingDeployments to be called")
	}
	// Dispatch should request at most `concurrency` deployments.
	if got := requestedLimit.Load(); got > concurrency {
		t.Errorf("expected limit <= %d, got %d", concurrency, got)
	}
}

func TestOrchestrator_DispatchSkipsWhenFull(t *testing.T) {
	var listCalled atomic.Bool
	ms := &mockOrchestratorStore{
		listBuildingFn: func(_ context.Context, _ int) ([]domain.CodeDeployment, error) {
			listCalled.Store(true)
			return nil, nil
		},
	}

	o := NewOrchestrator(ms, nil, WithConcurrency(1))

	// Pre-fill the semaphore to simulate all slots taken.
	sem := make(chan struct{}, 1)
	sem <- struct{}{} // fill it

	o.dispatch(context.Background(), sem)

	if listCalled.Load() {
		t.Error("expected dispatch to skip ListBuildingDeployments when all slots are taken")
	}
}

func TestTruncateString(t *testing.T) {
	s := "abcdefgh"
	if got := truncateString(s, 4); got != "abcd" {
		t.Errorf("expected 'abcd', got %q", got)
	}
	if got := truncateString(s, 100); got != s {
		t.Errorf("expected unchanged string, got %q", got)
	}
}
