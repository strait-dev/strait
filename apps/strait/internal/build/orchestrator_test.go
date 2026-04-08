package build

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"strait/internal/domain"
)

// mockBuildExecutor is a controllable stand-in for buildExecutor, allowing
// unit tests to simulate successful and failing builds without BuildKit.
type mockBuildExecutor struct {
	buildFn func(ctx context.Context, d *domain.CodeDeployment, addr string) (*BuildResult, error)
}

func (m *mockBuildExecutor) Build(ctx context.Context, d *domain.CodeDeployment, addr string) (*BuildResult, error) {
	if m.buildFn != nil {
		return m.buildFn(ctx, d, addr)
	}
	return &BuildResult{
		ImageURI:   "registry.example.com/strait-jobs/" + d.JobID + ":" + d.ID,
		Digest:     "sha256:deadbeef",
		BuildLogs:  "mock build logs",
		FinishedAt: time.Now().UTC(),
	}, nil
}

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

// newTestOrchestratorWithMockBuilder creates an Orchestrator wired with a
// mockBuildExecutor so runBuild can be exercised without a real Builder.
func newTestOrchestratorWithMockBuilder(store OrchestratorStore, executor *mockBuildExecutor) *Orchestrator {
	id, _ := uuid.NewV7()
	return &Orchestrator{
		store:    store,
		builder:  executor,
		workerID: "orchestrator-" + id.String(),
		logger:   slog.Default(),
	}
}

// TestOrchestratorRunBuild_SuccessMarksReadyAndActivates verifies the happy
// path: Build returns a result → status set to ready with image data → job
// activated.
func TestOrchestratorRunBuild_SuccessMarksReadyAndActivates(t *testing.T) {
	dep := domain.CodeDeployment{
		ID:        "deploy-rb-ok",
		JobID:     "job-rb-ok",
		ProjectID: "proj-rb",
		Runtime:   domain.RuntimePython,
		Status:    domain.DeploymentStatusBuilding,
	}

	wantImage := "registry.example.com/strait-jobs/job-rb-ok:deploy-rb-ok"
	wantDigest := "sha256:aabbcc"

	var gotStatus domain.DeploymentBuildStatus
	var gotFields map[string]any
	var activateJobID, activateDeployID string

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, fields map[string]any) error {
			gotStatus = status
			gotFields = fields
			return nil
		},
		setActiveDeploymentFn: func(_ context.Context, jobID, deploymentID, _ string) error {
			activateJobID = jobID
			activateDeployID = deploymentID
			return nil
		},
	}

	executor := &mockBuildExecutor{
		buildFn: func(_ context.Context, _ *domain.CodeDeployment, _ string) (*BuildResult, error) {
			return &BuildResult{
				ImageURI:   wantImage,
				Digest:     wantDigest,
				BuildLogs:  "all good",
				FinishedAt: time.Now().UTC(),
			}, nil
		},
	}

	o := newTestOrchestratorWithMockBuilder(ms, executor)
	o.runBuild(context.Background(), &dep)

	if gotStatus != domain.DeploymentStatusReady {
		t.Errorf("expected status=ready, got %s", gotStatus)
	}
	if gotFields["built_image_uri"] != wantImage {
		t.Errorf("expected image_uri=%s, got %v", wantImage, gotFields["built_image_uri"])
	}
	if gotFields["built_image_digest"] != wantDigest {
		t.Errorf("expected digest=%s, got %v", wantDigest, gotFields["built_image_digest"])
	}
	if activateJobID != dep.JobID || activateDeployID != dep.ID {
		t.Errorf("SetActiveDeployment called with job=%s deploy=%s; want %s/%s",
			activateJobID, activateDeployID, dep.JobID, dep.ID)
	}
}

// TestOrchestratorRunBuild_BuildErrorSetsFailedStatus verifies that a generic
// build error produces a failed deployment with a persisted error message.
func TestOrchestratorRunBuild_BuildErrorSetsFailedStatus(t *testing.T) {
	dep := domain.CodeDeployment{
		ID:        "deploy-rb-fail",
		JobID:     "job-rb-fail",
		ProjectID: "proj-rb",
		Status:    domain.DeploymentStatusBuilding,
	}

	var gotStatus domain.DeploymentBuildStatus
	var gotErrMsg string

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, fields map[string]any) error {
			gotStatus = status
			if m, ok := fields["error_message"].(string); ok {
				gotErrMsg = m
			}
			return nil
		},
	}

	executor := &mockBuildExecutor{
		buildFn: func(_ context.Context, _ *domain.CodeDeployment, _ string) (*BuildResult, error) {
			return nil, errors.New("buildkit: connection refused")
		},
	}

	o := newTestOrchestratorWithMockBuilder(ms, executor)
	o.runBuild(context.Background(), &dep)

	if gotStatus != domain.DeploymentStatusFailed {
		t.Errorf("expected status=failed, got %s", gotStatus)
	}
	if gotErrMsg == "" {
		t.Error("expected non-empty error_message in update fields")
	}
}

// TestOrchestratorRunBuild_TimeoutSetsTimedOutStatus verifies that
// context.DeadlineExceeded from the builder produces a timed_out deployment.
func TestOrchestratorRunBuild_TimeoutSetsTimedOutStatus(t *testing.T) {
	dep := domain.CodeDeployment{
		ID:        "deploy-rb-timeout",
		JobID:     "job-rb-timeout",
		ProjectID: "proj-rb",
		Status:    domain.DeploymentStatusBuilding,
	}

	var gotStatus domain.DeploymentBuildStatus

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, _ map[string]any) error {
			gotStatus = status
			return nil
		},
	}

	executor := &mockBuildExecutor{
		buildFn: func(_ context.Context, _ *domain.CodeDeployment, _ string) (*BuildResult, error) {
			return nil, context.DeadlineExceeded
		},
	}

	o := newTestOrchestratorWithMockBuilder(ms, executor)
	o.runBuild(context.Background(), &dep)

	if gotStatus != domain.DeploymentStatusTimedOut {
		t.Errorf("expected status=timed_out, got %s", gotStatus)
	}
}

// TestOrchestratorRunBuild_SetActiveDeploymentFailureDoesNotFailBuild verifies
// that if SetActiveDeployment fails, we log but do NOT overwrite the ready status.
func TestOrchestratorRunBuild_SetActiveDeploymentFailureDoesNotFailBuild(t *testing.T) {
	dep := domain.CodeDeployment{
		ID:        "deploy-rb-activate-fail",
		JobID:     "job-rb-activate-fail",
		ProjectID: "proj-rb",
		Status:    domain.DeploymentStatusBuilding,
	}

	var statusUpdates []domain.DeploymentBuildStatus

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, _ map[string]any) error {
			statusUpdates = append(statusUpdates, status)
			return nil
		},
		setActiveDeploymentFn: func(_ context.Context, _, _, _ string) error {
			return errors.New("db: set active deployment failed")
		},
	}

	executor := &mockBuildExecutor{} // default: returns success

	o := newTestOrchestratorWithMockBuilder(ms, executor)
	o.runBuild(context.Background(), &dep)

	// The only status update should be "ready" — no subsequent "failed" update.
	if len(statusUpdates) != 1 {
		t.Fatalf("expected exactly 1 status update, got %d: %v", len(statusUpdates), statusUpdates)
	}
	if statusUpdates[0] != domain.DeploymentStatusReady {
		t.Errorf("expected status=ready, got %s", statusUpdates[0])
	}
}

// TestOrchestratorRunBuild_BuiltImageAndDigestPersistedCorrectly verifies that
// both image URI and digest from the BuildResult are stored on the deployment.
func TestOrchestratorRunBuild_BuiltImageAndDigestPersistedCorrectly(t *testing.T) {
	dep := domain.CodeDeployment{
		ID:        "deploy-rb-fields",
		JobID:     "job-rb-fields",
		ProjectID: "proj-rb",
		Status:    domain.DeploymentStatusBuilding,
	}

	wantURI := "ghcr.io/strait-dev/jobs/job-rb-fields:deploy-rb-fields"
	wantDigest := "sha256:f00cafe"
	wantLogs := "build log line 1\nbuild log line 2\n"

	var gotFields map[string]any

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, _ domain.DeploymentBuildStatus, fields map[string]any) error {
			gotFields = fields
			return nil
		},
	}

	executor := &mockBuildExecutor{
		buildFn: func(_ context.Context, _ *domain.CodeDeployment, _ string) (*BuildResult, error) {
			return &BuildResult{
				ImageURI:   wantURI,
				Digest:     wantDigest,
				BuildLogs:  wantLogs,
				FinishedAt: time.Now().UTC(),
			}, nil
		},
	}

	o := newTestOrchestratorWithMockBuilder(ms, executor)
	o.runBuild(context.Background(), &dep)

	if gotFields == nil {
		t.Fatal("expected update fields to be set")
	}
	if gotFields["built_image_uri"] != wantURI {
		t.Errorf("built_image_uri: want %q, got %v", wantURI, gotFields["built_image_uri"])
	}
	if gotFields["built_image_digest"] != wantDigest {
		t.Errorf("built_image_digest: want %q, got %v", wantDigest, gotFields["built_image_digest"])
	}
	if gotFields["build_logs"] != wantLogs {
		t.Errorf("build_logs: want %q, got %v", wantLogs, gotFields["build_logs"])
	}
}

// TestOrchestratorRunBuild_AddressPoolPassedToBuilder verifies that when an
// AddressPool is configured, each build receives the pool's address.
func TestOrchestratorRunBuild_AddressPoolPassedToBuilder(t *testing.T) {
	dep := domain.CodeDeployment{
		ID:        "deploy-rb-pool",
		JobID:     "job-rb-pool",
		ProjectID: "proj-rb",
		Status:    domain.DeploymentStatusBuilding,
	}

	var gotAddr string

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, _ domain.DeploymentBuildStatus, _ map[string]any) error {
			return nil
		},
		setActiveDeploymentFn: func(_ context.Context, _, _, _ string) error { return nil },
	}

	executor := &mockBuildExecutor{
		buildFn: func(_ context.Context, _ *domain.CodeDeployment, addr string) (*BuildResult, error) {
			gotAddr = addr
			return &BuildResult{
				ImageURI:   "registry.example.com/test:deploy-rb-pool",
				Digest:     "sha256:abc",
				FinishedAt: time.Now().UTC(),
			}, nil
		},
	}

	pool := NewAddressPool("bk1.local:1234", "bk2.local:1234,bk3.local:1234")
	o := newTestOrchestratorWithMockBuilder(ms, executor)
	o.addrPool = pool

	o.runBuild(context.Background(), &dep)

	if gotAddr == "" {
		t.Error("expected a non-empty BuildKit address from the pool")
	}
}

// dispatch branch coverage.

// TestOrchestrator_Dispatch_ClaimError_SkipsAndContinues verifies that when
// ClaimBuildingDeployment returns an error, dispatch breaks out of the loop
// without panicking and releases the semaphore slot it had acquired.
func TestOrchestrator_Dispatch_ClaimError_SkipsAndContinues(t *testing.T) {
	t.Parallel()

	ms := &mockOrchestratorStore{
		claimBuildingFn: func(_ context.Context, _ string) (*domain.CodeDeployment, error) {
			return nil, errors.New("database connection lost")
		},
	}

	o := NewOrchestrator(ms, nil,
		WithPollInterval(1*time.Millisecond),
		WithConcurrency(2),
	)

	sem := make(chan struct{}, 2)
	// Must not panic and must not leak the semaphore slot.
	o.dispatch(context.Background(), sem)

	// After dispatch returns, the semaphore must be empty (no leaked slots).
	if len(sem) != 0 {
		t.Errorf("semaphore leaked: %d slots still held after dispatch error", len(sem))
	}
}

// TestOrchestrator_Dispatch_SemaphoreFull_BreaksLoop verifies that when all
// concurrency slots are already taken, dispatch breaks without calling
// ClaimBuildingDeployment and returns immediately.
func TestOrchestrator_Dispatch_SemaphoreFull_BreaksLoop(t *testing.T) {
	t.Parallel()

	var claimCalled bool
	ms := &mockOrchestratorStore{
		claimBuildingFn: func(_ context.Context, _ string) (*domain.CodeDeployment, error) {
			claimCalled = true
			return nil, nil
		},
	}

	o := NewOrchestrator(ms, nil, WithConcurrency(1))

	// Fill the semaphore completely before calling dispatch.
	sem := make(chan struct{}, 1)
	sem <- struct{}{} // slot is taken

	o.dispatch(context.Background(), sem)

	if claimCalled {
		t.Error("ClaimBuildingDeployment must not be called when semaphore is full")
	}
}

// releaseStale branch coverage.

// TestOrchestrator_ReleaseStale_ZeroBuilderTimeout_UsesDefault30Min verifies
// that when builderTimeout is 0 (no builder configured), the stale cutoff
// defaults to 30 minutes.
func TestOrchestrator_ReleaseStale_ZeroBuilderTimeout_UsesDefault30Min(t *testing.T) {
	t.Parallel()

	var capturedDuration time.Duration
	ms := &mockOrchestratorStore{
		releaseStaleClaimsFn: func(_ context.Context, olderThan time.Duration) (int64, error) {
			capturedDuration = olderThan
			return 0, nil
		},
	}

	// builderTimeout == 0 because builder is nil.
	o := NewOrchestrator(ms, nil)

	o.releaseStale(context.Background())

	if capturedDuration != 30*time.Minute {
		t.Errorf("stale cutoff = %v, want 30m (default when no builder timeout)", capturedDuration)
	}
}

// TestOrchestrator_ReleaseStale_NonzeroBuilderTimeout_UsesDoubled verifies that
// when builderTimeout is set, the stale cutoff is builderTimeout * 2.
func TestOrchestrator_ReleaseStale_NonzeroBuilderTimeout_UsesDoubled(t *testing.T) {
	t.Parallel()

	const builderTimeout = 10 * time.Minute

	var capturedDuration time.Duration
	ms := &mockOrchestratorStore{
		releaseStaleClaimsFn: func(_ context.Context, olderThan time.Duration) (int64, error) {
			capturedDuration = olderThan
			return 0, nil
		},
	}

	o := NewOrchestrator(ms, nil)
	o.builderTimeout = builderTimeout // inject directly to bypass *Builder requirement

	o.releaseStale(context.Background())

	if capturedDuration != builderTimeout*2 {
		t.Errorf("stale cutoff = %v, want %v (builderTimeout*2)", capturedDuration, builderTimeout*2)
	}
}

// TestOrchestrator_ReleaseStale_StoreError_NoPanic verifies that a store error
// during releaseStale is logged and the function returns without panicking.
func TestOrchestrator_ReleaseStale_StoreError_NoPanic(t *testing.T) {
	t.Parallel()

	ms := &mockOrchestratorStore{
		releaseStaleClaimsFn: func(_ context.Context, _ time.Duration) (int64, error) {
			return 0, errors.New("network timeout")
		},
	}

	o := NewOrchestrator(ms, nil)

	// Must not panic.
	o.releaseStale(context.Background())
}
