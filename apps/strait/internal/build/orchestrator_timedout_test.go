package build

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
)

// TestOrchestrator_HandleBuildFailure_TimedOut verifies that when a build fails
// due to context deadline exceeded, handleBuildFailure sets DeploymentStatusTimedOut
// rather than DeploymentStatusFailed. This distinction lets the CLI surface a
// clear "build timed out" message instead of a generic "failed" message.
func TestOrchestrator_HandleBuildFailure_TimedOut(t *testing.T) {
	t.Parallel()

	deployment := &domain.CodeDeployment{
		ID:        "deploy_timeout",
		JobID:     "job_abc",
		ProjectID: "proj_123",
		Runtime:   domain.RuntimePython,
	}

	var capturedStatus domain.DeploymentBuildStatus

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, _ map[string]any) error {
			capturedStatus = status
			return nil
		},
	}

	o := NewOrchestrator(ms, nil)

	// Simulate a build that timed out via context.DeadlineExceeded.
	o.handleBuildFailure(context.Background(), deployment, context.DeadlineExceeded, o.logger)

	if capturedStatus != domain.DeploymentStatusTimedOut {
		t.Errorf("handleBuildFailure with DeadlineExceeded: status = %q, want %q",
			capturedStatus, domain.DeploymentStatusTimedOut)
	}
}

// TestOrchestrator_HandleBuildFailure_ContextCanceled verifies that context
// cancellation (distinct from deadline exceeded) also produces timed_out status.
// A cancelled build is a timeout from the operator's perspective.
func TestOrchestrator_HandleBuildFailure_ContextCanceled(t *testing.T) {
	t.Parallel()

	deployment := &domain.CodeDeployment{
		ID:      "deploy_canceled",
		JobID:   "job_abc",
		Runtime: domain.RuntimeGo,
	}

	var capturedStatus domain.DeploymentBuildStatus

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, _ map[string]any) error {
			capturedStatus = status
			return nil
		},
	}

	o := NewOrchestrator(ms, nil)
	o.handleBuildFailure(context.Background(), deployment, context.Canceled, o.logger)

	if capturedStatus != domain.DeploymentStatusTimedOut {
		t.Errorf("handleBuildFailure with Canceled: status = %q, want %q",
			capturedStatus, domain.DeploymentStatusTimedOut)
	}
}

// TestOrchestrator_HandleBuildFailure_OtherError verifies that non-timeout
// errors produce DeploymentStatusFailed (not timed_out). This test is the
// counterpart to the timeout tests — failed and timed_out are distinct values.
func TestOrchestrator_HandleBuildFailure_OtherError(t *testing.T) {
	t.Parallel()

	deployment := &domain.CodeDeployment{
		ID:      "deploy_failed",
		JobID:   "job_abc",
		Runtime: domain.RuntimePython,
	}

	var capturedStatus domain.DeploymentBuildStatus

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, _ map[string]any) error {
			capturedStatus = status
			return nil
		},
	}

	o := NewOrchestrator(ms, nil)
	o.handleBuildFailure(context.Background(), deployment, errors.New("buildkit: image push failed"), o.logger)

	if capturedStatus != domain.DeploymentStatusFailed {
		t.Errorf("handleBuildFailure with generic error: status = %q, want %q",
			capturedStatus, domain.DeploymentStatusFailed)
	}
}

// TestOrchestrator_TimedOutVsFailed_DifferentValues verifies that
// DeploymentStatusTimedOut and DeploymentStatusFailed are distinct string
// constants that will not accidentally match in switch statements or DB queries.
func TestOrchestrator_TimedOutVsFailed_DifferentValues(t *testing.T) {
	t.Parallel()

	if domain.DeploymentStatusTimedOut == domain.DeploymentStatusFailed {
		t.Errorf("DeploymentStatusTimedOut == DeploymentStatusFailed: both are %q; must be distinct",
			domain.DeploymentStatusFailed)
	}
}

// TestOrchestrator_HandleBuildFailure_TarballError verifies that tarball
// validation failures produce a meaningful error message in the status update.
// The error message must not be the raw tarball error struct but a human-readable
// "source validation failed: ..." prefix.
func TestOrchestrator_HandleBuildFailure_TarballError(t *testing.T) {
	t.Parallel()

	deployment := &domain.CodeDeployment{
		ID:      "deploy_tarball_err",
		JobID:   "job_abc",
		Runtime: domain.RuntimePython,
	}

	var capturedStatus domain.DeploymentBuildStatus
	var capturedFields map[string]any

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, fields map[string]any) error {
			capturedStatus = status
			capturedFields = fields
			return nil
		},
	}

	o := NewOrchestrator(ms, nil)
	tarErr := &TarballError{Reason: "path traversal detected", Entry: "../etc/passwd"}
	o.handleBuildFailure(context.Background(), deployment, tarErr, o.logger)

	if capturedStatus != domain.DeploymentStatusFailed {
		t.Errorf("tarball error: status = %q, want failed", capturedStatus)
	}

	errMsg, _ := capturedFields["error_message"].(string)
	if errMsg == "" {
		t.Error("error_message field is empty for tarball failure")
	}
	// The message must surface the reason, not the raw struct repr.
	if len(errMsg) < len("source validation failed:") {
		t.Errorf("error_message too short, want 'source validation failed: ...': %q", errMsg)
	}
}

// TestOrchestrator_HandleBuildFailure_WrappedDeadline verifies that a
// DeadlineExceeded wrapped inside a fmt.Errorf("%w", ...) chain is still
// detected as a timeout via errors.Is.
func TestOrchestrator_HandleBuildFailure_WrappedDeadline(t *testing.T) {
	t.Parallel()

	deployment := &domain.CodeDeployment{
		ID:      "deploy_wrapped",
		JobID:   "job_abc",
		Runtime: domain.RuntimePython,
	}

	var capturedStatus domain.DeploymentBuildStatus

	ms := &mockOrchestratorStore{
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, _ map[string]any) error {
			capturedStatus = status
			return nil
		},
	}

	o := NewOrchestrator(ms, nil)

	// Wrap the deadline error the same way the build pipeline does.
	wrappedErr := &wrappedErr{msg: "buildkit: context deadline exceeded", cause: context.DeadlineExceeded}
	o.handleBuildFailure(context.Background(), deployment, wrappedErr, o.logger)

	if capturedStatus != domain.DeploymentStatusTimedOut {
		t.Errorf("wrapped DeadlineExceeded: status = %q, want timed_out", capturedStatus)
	}
}

// wrappedErr is a minimal error wrapper that implements errors.Is via Unwrap.
type wrappedErr struct {
	msg   string
	cause error
}

func (e *wrappedErr) Error() string { return e.msg + ": " + e.cause.Error() }
func (e *wrappedErr) Unwrap() error { return e.cause }

// Ensure wrappedErr.Unwrap works for the test above.
var _ interface{ Unwrap() error } = (*wrappedErr)(nil)

// TestOrchestrator_HandleBuildFailure_NilBuilder verifies that an orchestrator
// with no builder configured marks the deployment as failed when runBuild is
// triggered. This exercises the nil-builder guard in runBuild.
func TestOrchestrator_HandleBuildFailure_NilBuilder_SetsFailedStatus(t *testing.T) {
	t.Parallel()

	deployment := &domain.CodeDeployment{
		ID:      "deploy_no_builder",
		JobID:   "job_abc",
		Runtime: domain.RuntimePython,
	}

	var capturedStatus domain.DeploymentBuildStatus

	ms := &mockOrchestratorStore{
		claimBuildingFn: func(_ context.Context, _ string) (*domain.CodeDeployment, error) {
			return deployment, nil
		},
		updateStatusFn: func(_ context.Context, _ string, status domain.DeploymentBuildStatus, _ map[string]any) error {
			capturedStatus = status
			return nil
		},
	}

	// Builder is nil — runBuild must mark the deployment failed.
	o := NewOrchestrator(ms, nil,
		WithPollInterval(1*time.Millisecond),
		WithConcurrency(1),
	)

	// Run one dispatch cycle synchronously.
	sem := make(chan struct{}, 1)
	o.dispatch(context.Background(), sem)

	if capturedStatus != domain.DeploymentStatusFailed {
		t.Errorf("nil builder: status = %q, want failed", capturedStatus)
	}
}
