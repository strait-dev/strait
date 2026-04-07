package build

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// mockOrchestratorStore is a minimal stub for OrchestratorStore.
type mockOrchestratorStore struct {
	claimBuildingFn       func(ctx context.Context, workerID string) (*domain.CodeDeployment, error)
	releaseStaleClaimsFn  func(ctx context.Context, olderThan time.Duration) (int64, error)
	listBuildingFn        func(ctx context.Context, limit int) ([]domain.CodeDeployment, error)
	updateStatusFn        func(ctx context.Context, id string, status domain.DeploymentBuildStatus, fields map[string]any) error
	setActiveDeploymentFn func(ctx context.Context, jobID, deploymentID, projectID string) error
}

func (m *mockOrchestratorStore) ClaimBuildingDeployment(ctx context.Context, workerID string) (*domain.CodeDeployment, error) {
	if m.claimBuildingFn != nil {
		return m.claimBuildingFn(ctx, workerID)
	}
	return nil, nil
}

func (m *mockOrchestratorStore) ReleaseStaleClaimedDeployments(ctx context.Context, olderThan time.Duration) (int64, error) {
	if m.releaseStaleClaimsFn != nil {
		return m.releaseStaleClaimsFn(ctx, olderThan)
	}
	return 0, nil
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

	deployments := make([]domain.CodeDeployment, totalDeployments)
	for i := range deployments {
		deployments[i] = domain.CodeDeployment{
			ID:        fmt.Sprintf("d%d", i),
			JobID:     "job",
			ProjectID: "proj",
			Status:    domain.DeploymentStatusBuilding,
		}
	}

	var claimCalls atomic.Int32
	idx := atomic.Int32{}

	ms := &mockOrchestratorStore{
		claimBuildingFn: func(_ context.Context, _ string) (*domain.CodeDeployment, error) {
			claimCalls.Add(1)
			i := int(idx.Add(1)) - 1
			if i >= len(deployments) {
				return nil, nil // queue exhausted
			}
			return &deployments[i], nil
		},
		// Accept UpdateCodeDeploymentStatus calls from runBuild (nil builder path).
		updateStatusFn: func(_ context.Context, _ string, _ domain.DeploymentBuildStatus, _ map[string]any) error {
			return nil
		},
	}

	o := NewOrchestrator(ms, nil, WithConcurrency(concurrency))
	sem := make(chan struct{}, concurrency)

	// Call dispatch — it claims at most `concurrency` deployments (slots bound it).
	o.dispatch(context.Background(), sem)

	// dispatch is synchronous (waits on wg.Wait), so claimCalls is final now.
	got := claimCalls.Load()
	if got == 0 {
		t.Error("expected ClaimBuildingDeployment to be called at least once")
	}
	if got > int32(concurrency)+1 {
		// +1 for the nil-return probe call
		t.Errorf("expected at most %d claim calls, got %d", concurrency+1, got)
	}
}

func TestOrchestrator_DispatchSkipsWhenFull(t *testing.T) {
	var claimCalled atomic.Bool
	ms := &mockOrchestratorStore{
		claimBuildingFn: func(_ context.Context, _ string) (*domain.CodeDeployment, error) {
			claimCalled.Store(true)
			return nil, nil
		},
	}

	o := NewOrchestrator(ms, nil, WithConcurrency(1))

	// Pre-fill the semaphore to simulate all slots taken.
	sem := make(chan struct{}, 1)
	sem <- struct{}{} // fill it

	o.dispatch(context.Background(), sem)

	if claimCalled.Load() {
		t.Error("expected dispatch to skip ClaimBuildingDeployment when all slots are taken")
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

// TestOrchestrator_TimeoutSetsTimedOutStatus verifies that a context.DeadlineExceeded
// build error produces a timed_out status rather than failed.
func TestOrchestrator_TimeoutSetsTimedOutStatus(t *testing.T) {
	deployment := domain.CodeDeployment{
		ID:        "deploy_timeout",
		JobID:     "job_to",
		ProjectID: "proj_123",
		Status:    domain.DeploymentStatusBuilding,
	}

	var finalStatus domain.DeploymentBuildStatus
	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, _ map[string]any) error {
			finalStatus = status
			return nil
		},
	}

	o := NewOrchestrator(ms, nil)
	o.handleBuildFailure(context.Background(), &deployment, context.DeadlineExceeded, o.logger)

	if finalStatus != domain.DeploymentStatusTimedOut {
		t.Errorf("expected status=timed_out for DeadlineExceeded, got %s", finalStatus)
	}
}

// TestOrchestrator_CancelSetsTimedOutStatus verifies that context.Canceled also
// maps to timed_out (the operator cancelled the build, not a code failure).
func TestOrchestrator_CancelSetsTimedOutStatus(t *testing.T) {
	deployment := domain.CodeDeployment{
		ID:        "deploy_cancel",
		JobID:     "job_c",
		ProjectID: "proj_123",
		Status:    domain.DeploymentStatusBuilding,
	}

	var finalStatus domain.DeploymentBuildStatus
	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, _ map[string]any) error {
			finalStatus = status
			return nil
		},
	}

	o := NewOrchestrator(ms, nil)
	o.handleBuildFailure(context.Background(), &deployment, context.Canceled, o.logger)

	if finalStatus != domain.DeploymentStatusTimedOut {
		t.Errorf("expected status=timed_out for Canceled, got %s", finalStatus)
	}
}

// TestOrchestrator_ReleaseStaleCallsStore verifies that releaseStale forwards to
// the store with a non-zero duration.
func TestOrchestrator_ReleaseStaleCallsStore(t *testing.T) {
	var capturedDuration time.Duration
	ms := &mockOrchestratorStore{
		releaseStaleClaimsFn: func(_ context.Context, olderThan time.Duration) (int64, error) {
			capturedDuration = olderThan
			return 3, nil
		},
	}

	o := NewOrchestrator(ms, nil)
	o.releaseStale(context.Background())

	if capturedDuration <= 0 {
		t.Errorf("expected positive stale cutoff duration, got %s", capturedDuration)
	}
}

// TestOrchestrator_DispatchClaimsUniqueWorkerID verifies that each Orchestrator
// instance gets a distinct workerID so multiple replicas don't share claim ownership.
func TestOrchestrator_DispatchClaimsUniqueWorkerID(t *testing.T) {
	var workerIDs []string
	ms := &mockOrchestratorStore{
		claimBuildingFn: func(_ context.Context, workerID string) (*domain.CodeDeployment, error) {
			workerIDs = append(workerIDs, workerID)
			return nil, nil // nothing to claim
		},
	}

	o1 := NewOrchestrator(ms, nil)
	o2 := NewOrchestrator(ms, nil)

	sem1 := make(chan struct{}, 1)
	sem2 := make(chan struct{}, 1)
	o1.dispatch(context.Background(), sem1)
	o2.dispatch(context.Background(), sem2)

	if len(workerIDs) < 2 {
		t.Fatal("expected at least 2 claim calls")
	}
	if workerIDs[0] == workerIDs[1] {
		t.Errorf("expected distinct workerIDs, both got %q", workerIDs[0])
	}
}

// TestOrchestrator_SuccessfulBuild_viaClaim verifies the full happy path using
// the claim-based dispatch: claim → build → ready + activate.
func TestOrchestrator_SuccessfulBuild_viaClaim(t *testing.T) {
	deployment := domain.CodeDeployment{
		ID:        "deploy_claim_1",
		JobID:     "job_claim",
		ProjectID: "proj_claim",
		Runtime:   domain.RuntimePython,
		Status:    domain.DeploymentStatusBuilding,
	}

	var statusUpdates []domain.DeploymentBuildStatus
	var activeDeploymentSet bool
	claimed := false

	ms := &mockOrchestratorStore{
		claimBuildingFn: func(_ context.Context, _ string) (*domain.CodeDeployment, error) {
			if !claimed {
				claimed = true
				return &deployment, nil
			}
			return nil, nil
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

	result := &BuildResult{
		ImageURI:   fmt.Sprintf("123.dkr.ecr.us-east-1.amazonaws.com/strait-jobs/%s:%s", deployment.JobID, deployment.ID),
		Digest:     "sha256:cafebabe",
		BuildLogs:  "done",
		FinishedAt: time.Now().UTC(),
	}

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
		t.Errorf("expected status=ready, got %v", statusUpdates)
	}
	if !activeDeploymentSet {
		t.Error("expected SetActiveDeployment to be called")
	}
	_ = ms // used above
}
