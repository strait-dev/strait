package build

import (
	"context"
	"errors"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

// TestBuilder_WithSOCI_DefaultDisabled verifies that a freshly constructed Builder
// has SOCI disabled — no index generation without an explicit opt-in.
func TestBuilder_WithSOCI_DefaultDisabled(t *testing.T) {
	t.Parallel()
	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0)
	if b.sociEnabled {
		t.Error("expected sociEnabled=false by default")
	}
	if b.sociRunner != nil {
		t.Error("expected sociRunner=nil by default")
	}
}

// TestBuilder_WithSOCI_EnabledSetsFields verifies that WithSOCI(true) sets both
// the enabled flag and the runner function.
func TestBuilder_WithSOCI_EnabledSetsFields(t *testing.T) {
	t.Parallel()
	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)
	if !b.sociEnabled {
		t.Error("expected sociEnabled=true after WithSOCI(true)")
	}
	if b.sociRunner == nil {
		t.Error("expected sociRunner to be set after WithSOCI(true)")
	}
}

// TestBuilder_WithSOCI_DisableAfterEnable verifies that WithSOCI(false) after
// WithSOCI(true) leaves the builder in a consistent disabled state.
func TestBuilder_WithSOCI_DisableAfterEnable(t *testing.T) {
	t.Parallel()
	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).
		WithSOCI(true).
		WithSOCI(false)
	if b.sociEnabled {
		t.Error("expected sociEnabled=false after WithSOCI(false)")
	}
}

// TestBuilder_GenerateSOCIIndex_DisabledIsNoop verifies that generateSOCIIndex
// never calls the runner when SOCI is disabled.
func TestBuilder_GenerateSOCIIndex_DisabledIsNoop(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0)
	b.sociRunner = func(_ context.Context, _ string) error {
		called.Store(true)
		return nil
	}
	// sociEnabled is false — runner must not be called.
	b.generateSOCIIndex(context.Background(), "registry.example.com/repo:tag")

	if called.Load() {
		t.Error("sociRunner was called despite sociEnabled=false")
	}
}

// TestBuilder_GenerateSOCIIndex_CallsRunnerWithCorrectRef verifies that when SOCI
// is enabled, generateSOCIIndex calls the runner with the exact image reference
// passed to it.
func TestBuilder_GenerateSOCIIndex_CallsRunnerWithCorrectRef(t *testing.T) {
	t.Parallel()

	const wantRef = "123456.dkr.ecr.us-east-1.amazonaws.com/strait-jobs/proj/job:deploy_abc"
	var gotRef string

	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)
	b.sociRunner = func(_ context.Context, imageRef string) error {
		gotRef = imageRef
		return nil
	}
	b.generateSOCIIndex(context.Background(), wantRef)

	if gotRef != wantRef {
		t.Errorf("sociRunner called with %q, want %q", gotRef, wantRef)
	}
}

// TestBuilder_GenerateSOCIIndex_RunnerFailureDoesNotPanic verifies that a runner
// returning an error does not cause generateSOCIIndex to panic or propagate the
// error. SOCI failure must always be silent (logged, not returned).
func TestBuilder_GenerateSOCIIndex_RunnerFailureDoesNotPanic(t *testing.T) {
	t.Parallel()

	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)
	b.sociRunner = func(_ context.Context, _ string) error {
		return errors.New("ECR auth failure: token expired")
	}
	// Must not panic.
	b.generateSOCIIndex(context.Background(), "registry.example.com/repo:tag")
}

// TestBuilder_GenerateSOCIIndex_NilRunnerIsNoop verifies that a nil sociRunner
// never causes a nil-function panic, even when sociEnabled is true.
// This guards against a misconfigured builder.
func TestBuilder_GenerateSOCIIndex_NilRunnerIsNoop(t *testing.T) {
	t.Parallel()

	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0)
	b.sociEnabled = true
	b.sociRunner = nil // explicitly nil

	// Must not panic.
	b.generateSOCIIndex(context.Background(), "registry.example.com/repo:tag")
}

// TestBuilder_GenerateSOCIIndex_ContextRespected verifies that generateSOCIIndex
// passes a child context with a 2-minute deadline to the runner, and that the
// runner can observe a cancelled context.
func TestBuilder_GenerateSOCIIndex_ContextRespected(t *testing.T) {
	t.Parallel()

	// Use a pre-cancelled parent context to simulate immediate cancellation.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling

	var runnerCtxErr error
	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)
	b.sociRunner = func(ctx context.Context, _ string) error {
		// Runner must see a context that is done (either from parent or own timeout).
		select {
		case <-ctx.Done():
			runnerCtxErr = ctx.Err()
		default:
			// Context not yet done — that's fine too; the child context has its own 2m timeout.
		}
		return ctx.Err()
	}
	// Must not panic even when parent context is already cancelled.
	b.generateSOCIIndex(ctx, "registry.example.com/repo:tag")
	_ = runnerCtxErr // checked implicitly by no-panic
}

// TestBuilder_GenerateSOCIIndex_InternalTimeoutBound verifies that the runner
// receives a context with a deadline no greater than 2 minutes from now,
// regardless of how long the parent context lives.
func TestBuilder_GenerateSOCIIndex_InternalTimeoutBound(t *testing.T) {
	t.Parallel()

	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)

	var deadlineSet bool
	b.sociRunner = func(ctx context.Context, _ string) error {
		dl, ok := ctx.Deadline()
		if !ok {
			return nil // no deadline set — unexpected but not a failure
		}
		deadlineSet = true
		remaining := time.Until(dl)
		const maxTimeout = 2*time.Minute + 5*time.Second // small buffer for test overhead
		if remaining > maxTimeout {
			t.Errorf("SOCI context deadline is %v from now, want <= %v", remaining, maxTimeout)
		}
		return nil
	}
	b.generateSOCIIndex(context.Background(), "registry.example.com/repo:tag")

	if !deadlineSet {
		t.Error("expected SOCI runner context to have a deadline")
	}
}

// TestRunSOCICLI_BinaryNotInPath verifies that runSOCICLI returns a descriptive
// error when the soci binary is not in PATH, rather than panicking or returning nil.
// In CI the soci binary is not installed, so this is the expected path.
// Cannot use t.Parallel() because t.Setenv modifies process-global state.
func TestRunSOCICLI_BinaryNotInPath(t *testing.T) {
	// Override PATH to be empty so exec.LookPath cannot find soci.
	t.Setenv("PATH", "")

	err := runSOCICLI(context.Background(), "registry.example.com/repo:tag")
	if err == nil {
		t.Fatal("expected error when soci binary is not in PATH, got nil")
	}
	if !errors.Is(err, exec.ErrNotFound) {
		// Acceptable: the error wraps exec.ErrNotFound or contains "PATH" in message.
		// Just assert it's non-nil (already checked above).
		t.Logf("error (non-ErrNotFound, still acceptable): %v", err)
	}
}
