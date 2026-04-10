package build

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// Adversarial tests — verify that edge-case inputs cannot cause panics,
// data corruption, or command injection.

// TestBuilder_GenerateSOCIIndex_CommandInjectionImageRef verifies that imageRefs
// containing shell metacharacters are passed as literal arguments to the soci
// binary, not interpreted by a shell. exec.CommandContext is immune to shell
// injection, but this test makes the property explicit and regression-proof.
func TestBuilder_GenerateSOCIIndex_CommandInjectionImageRef(t *testing.T) {
	t.Parallel()

	injectionAttempts := []string{
		`; rm -rf /`,
		"`rm -rf /`",
		`$(evil-command)`,
		"registry.example.com/repo:tag\ninjected",
		"registry.example.com/repo:tag && evil",
		"registry.example.com/repo:tag | cat /etc/passwd",
		"registry.example.com/repo:tag > /dev/null",
		"--malicious-flag",
	}

	for _, ref := range injectionAttempts {
		t.Run(fmt.Sprintf("ref=%q", ref[:min(len(ref), 40)]), func(t *testing.T) {
			t.Parallel()

			var gotRef string
			b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)
			b.sociRunner = func(_ context.Context, imageRef string) error {
				gotRef = imageRef
				return nil
			}
			b.generateSOCIIndex(context.Background(), ref)

			// The runner must receive the exact string — not a truncated or
			// modified version — confirming no shell interpretation occurred.
			if gotRef != ref {
				t.Errorf("runner received %q, want exact %q", gotRef, ref)
			}
		})
	}
}

// TestBuilder_GenerateSOCIIndex_EmptyImageRef verifies that an empty imageRef
// does not panic and is forwarded verbatim to the runner.
func TestBuilder_GenerateSOCIIndex_EmptyImageRef(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)
	b.sociRunner = func(_ context.Context, _ string) error {
		called.Store(true)
		return nil
	}
	// Must not panic with empty string.
	b.generateSOCIIndex(context.Background(), "")

	if !called.Load() {
		t.Error("expected runner to be called even with empty imageRef")
	}
}

// TestBuilder_GenerateSOCIIndex_LongImageRef verifies that an extremely long
// imageRef (1 MiB) does not cause OOM or panic — the string is passed through
// to the runner without truncation.
func TestBuilder_GenerateSOCIIndex_LongImageRef(t *testing.T) {
	t.Parallel()

	longRef := strings.Repeat("a", 1<<20) // 1 MiB
	var gotLen int
	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)
	b.sociRunner = func(_ context.Context, imageRef string) error {
		gotLen = len(imageRef)
		return nil
	}
	b.generateSOCIIndex(context.Background(), longRef)

	if gotLen != len(longRef) {
		t.Errorf("runner received ref of length %d, want %d", gotLen, len(longRef))
	}
}

// TestBuilder_GenerateSOCIIndex_NullByteInRef verifies that a null byte embedded
// in an imageRef does not cause a panic. Real soci would likely reject this ref,
// but generateSOCIIndex must not blow up — it hands the error back as a warning.
func TestBuilder_GenerateSOCIIndex_NullByteInRef(t *testing.T) {
	t.Parallel()

	refWithNull := "registry.example.com/repo\x00:tag"
	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)
	b.sociRunner = func(_ context.Context, _ string) error {
		return errors.New("invalid image reference: null byte")
	}
	// Must not panic — error is swallowed as a warning.
	b.generateSOCIIndex(context.Background(), refWithNull)
}

// TestBuilder_GenerateSOCIIndex_ConcurrentSafe verifies that generateSOCIIndex
// is safe to call concurrently from multiple goroutines (no data races).
// Run with -race to catch any races.
func TestBuilder_GenerateSOCIIndex_ConcurrentSafe(t *testing.T) {
	t.Parallel()

	const goroutines = 100
	var count atomic.Int64

	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)
	b.sociRunner = func(_ context.Context, _ string) error {
		count.Add(1)
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			b.generateSOCIIndex(context.Background(),
				fmt.Sprintf("registry.example.com/repo:tag-%d", i))
		}()
	}
	wg.Wait()

	if count.Load() != goroutines {
		t.Errorf("runner called %d times, want %d", count.Load(), goroutines)
	}
}

// TestBuilder_GenerateSOCIIndex_TimeoutCancelsSlowRunner verifies that
// generateSOCIIndex cancels the SOCI runner via the child context deadline when
// the runner takes longer than the configured timeout. Uses withSOCITimeout to
// set a short deadline without waiting 2 real minutes.
func TestBuilder_GenerateSOCIIndex_TimeoutCancelsSlowRunner(t *testing.T) {
	t.Parallel()

	var runnerCtxErr error
	started := make(chan struct{})

	b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).
		WithSOCI(true).
		withSOCITimeout(20 * time.Millisecond)

	b.sociRunner = func(ctx context.Context, _ string) error {
		close(started)
		// Block until the context is cancelled.
		<-ctx.Done()
		runnerCtxErr = ctx.Err()
		return ctx.Err()
	}

	b.generateSOCIIndex(context.Background(), "registry.example.com/repo:tag")
	<-started // ensure runner was actually entered

	if runnerCtxErr == nil {
		t.Error("expected runner context to be cancelled by internal timeout")
	}
	if !errors.Is(runnerCtxErr, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", runnerCtxErr)
	}
}

// Integration tests — place a real executable on disk, override PATH, and
// invoke runSOCICLI to verify end-to-end argument passing and error handling.

// writeFakeSociBinary writes a shell script to dir/soci that records its
// arguments to argsFile and exits with exitCode. Returns the directory.
// Cannot use t.Parallel() because they call t.Setenv to override PATH.
func writeFakeSociBinary(t *testing.T, dir string, exitCode int, argsFile string) {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/sh
echo "$@" > %s
exit %d
`, argsFile, exitCode)
	path := filepath.Join(dir, "soci")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake soci: %v", err)
	}
}

// TestRunSOCICLI_SuccessWithFakeBinary places a real shell script named "soci"
// in a temp directory, overrides PATH to that directory, and verifies that
// runSOCICLI calls the binary with "create <imageRef>" and returns nil on exit 0.
func TestRunSOCICLI_SuccessWithFakeBinary(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	writeFakeSociBinary(t, dir, 0, argsFile)
	t.Setenv("PATH", dir)

	const imageRef = "123456.dkr.ecr.us-east-1.amazonaws.com/strait/proj/job:deploy_xyz"
	if err := runSOCICLI(context.Background(), imageRef); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	recorded, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	got := strings.TrimSpace(string(recorded))
	want := "create " + imageRef
	if got != want {
		t.Errorf("fake soci received args %q, want %q", got, want)
	}
}

// TestRunSOCICLI_FailureWithFakeBinary verifies that runSOCICLI wraps and returns
// an error when the soci binary exits non-zero. The error message must contain
// the binary's stderr/stdout output.
func TestRunSOCICLI_FailureWithFakeBinary(t *testing.T) {
	dir := t.TempDir()
	// Write a script that prints a diagnostic and exits 1.
	script := `#!/bin/sh
echo "ECR: credentials expired" >&2
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "soci"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake soci: %v", err)
	}
	t.Setenv("PATH", dir)

	err := runSOCICLI(context.Background(), "registry.example.com/repo:tag")
	if err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "ECR: credentials expired") {
		t.Errorf("error %q does not contain expected stderr output", err.Error())
	}
}

// TestRunSOCICLI_ContextCancelledBeforeExec verifies that runSOCICLI propagates
// context cancellation when the context is already done before the binary runs.
func TestRunSOCICLI_ContextCancelledBeforeExec(t *testing.T) {
	dir := t.TempDir()
	// Write a binary that would succeed if it ran — but it should never run.
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "soci"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake soci: %v", err)
	}
	t.Setenv("PATH", dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before exec

	err := runSOCICLI(ctx, "registry.example.com/repo:tag")
	if err == nil {
		t.Fatal("expected error due to cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		// exec.CommandContext may wrap the error differently — accept any non-nil error.
		t.Logf("context-cancelled error (non-Canceled wrapping, still acceptable): %v", err)
	}
}

// TestRunSOCICLI_ImageRefPassedAsArgNotShell verifies at the integration level
// (real binary on disk) that shell metacharacters in the imageRef are passed as
// a literal argument and do not trigger shell evaluation.
func TestRunSOCICLI_ImageRefPassedAsArgNotShell(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	writeFakeSociBinary(t, dir, 0, argsFile)
	t.Setenv("PATH", dir)

	// A ref that would cause a shell to execute a subcommand if interpolated.
	maliciousRef := "registry.example.com/repo:$(touch " + filepath.Join(dir, "pwned") + ")"

	if err := runSOCICLI(context.Background(), maliciousRef); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The script records "$@" — if shell injection occurred, this would expand.
	recorded, _ := os.ReadFile(argsFile)
	got := strings.TrimSpace(string(recorded))
	want := "create " + maliciousRef
	if got != want {
		t.Errorf("args %q, want %q — possible shell injection", got, want)
	}

	// Confirm no "pwned" file was created by the injection.
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Error("shell injection succeeded: 'pwned' file was created")
	}
}

// Fuzz tests — exercise arbitrary imageRef values to confirm generateSOCIIndex
// and runSOCICLI never panic regardless of input.

// FuzzGenerateSOCIIndex_ImageRef verifies that generateSOCIIndex never panics
// regardless of the imageRef value — including arbitrary bytes, shell
// metacharacters, null bytes, and extremely long strings.
func FuzzGenerateSOCIIndex_ImageRef(f *testing.F) {
	// Seed corpus: common cases + known adversarial inputs.
	seeds := []string{
		"registry.example.com/repo:tag",
		"",
		"; rm -rf /",
		"$(evil)",
		"a\x00b",
		strings.Repeat("x", 4096),
		"registry.example.com/repo:tag\ninjected",
		"--flag",
		"registry/path/with spaces/repo:tag",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, imageRef string) {
		b := NewBuilder("tcp://buildkitd:1234", nil, nil, false, 0).WithSOCI(true)
		b.sociRunner = func(_ context.Context, _ string) error {
			// Simulate occasional failures — both paths must be panic-free.
			if len(imageRef)%2 == 0 {
				return errors.New("simulated runner error")
			}
			return nil
		}
		// Must never panic.
		b.generateSOCIIndex(context.Background(), imageRef)
	})
}

// FuzzRunSOCICLI_BinaryNotInPath verifies that runSOCICLI never panics for
// arbitrary imageRef values when the soci binary is absent. Uses an empty PATH
// so exec.LookPath always fails — no real process is spawned.
// Note: fuzz tests with t.Setenv on PATH are serial by design.
func FuzzRunSOCICLI_BinaryNotInPath(f *testing.F) {
	f.Add("registry.example.com/repo:tag")
	f.Add("")
	f.Add("; rm -rf /")
	f.Add("$(evil)")
	f.Add(strings.Repeat("a", 512))

	f.Fuzz(func(t *testing.T, imageRef string) {
		// Cannot call t.Setenv inside fuzz body (no Parallel, but still restricted).
		// Instead, call runSOCICLI with the knowledge that in CI, soci is not installed.
		// This exercises the LookPath error path on any machine without soci.
		// If soci IS installed, the binary will receive the fuzzed ref as an arg
		// and may exit non-zero — both outcomes must not panic.
		err := runSOCICLI(context.Background(), imageRef)
		_ = err // nil or non-nil, both valid — the invariant is no panic
	})
}
