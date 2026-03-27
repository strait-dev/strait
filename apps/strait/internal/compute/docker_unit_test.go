package compute

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------.
// NewDockerRuntime -- constructor
// ---------------------------------------------------------------------------.

func TestNewDockerRuntime_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	if rt == nil {
		t.Fatal("expected non-nil DockerRuntime")
	}
}

func TestNewDockerRuntime_ImplementsContainerRuntime(t *testing.T) {
	t.Parallel()
	var _ ContainerRuntime = NewDockerRuntime()
}

// ---------------------------------------------------------------------------.
// Create -- validation and error paths
// ---------------------------------------------------------------------------.

func TestDockerCreate_EmptyImage(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	_, err := rt.Create(context.Background(), RunRequest{ImageURI: ""})
	if err == nil {
		t.Fatal("expected error for empty image URI")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got: %v", err)
	}
}

func TestDockerCreate_InvalidImageURI(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	_, err := rt.Create(context.Background(), RunRequest{ImageURI: "image;rm -rf /"})
	if err == nil {
		t.Fatal("expected error for invalid image URI")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error for shell metachar, got: %v", err)
	}
}

func TestDockerCreate_InvalidEnvKey(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI: "nginx:latest",
		Env:      map[string]string{"BAD;KEY": "value"},
	})
	if err == nil {
		t.Fatal("expected error for invalid env key")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error for bad env key, got: %v", err)
	}
}

func TestDockerCreate_InvalidLabelKey(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI: "nginx:latest",
		Labels:   map[string]string{"bad;label": "value"},
	})
	if err == nil {
		t.Fatal("expected error for invalid label key")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error for bad label key, got: %v", err)
	}
}

func TestDockerCreate_CancelledContext(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.Create(ctx, RunRequest{ImageURI: "nginx:latest"})
	// With a cancelled context, docker run will fail (retryable error).
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// ---------------------------------------------------------------------------.
// Run -- validation and error paths
// ---------------------------------------------------------------------------.

func TestDockerRun_EmptyImage(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	_, err := rt.Run(context.Background(), RunRequest{ImageURI: ""})
	if err == nil {
		t.Fatal("expected error for empty image URI")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got: %v", err)
	}
}

func TestDockerRun_InvalidEnvKey(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	_, err := rt.Run(context.Background(), RunRequest{
		ImageURI: "nginx:latest",
		Env:      map[string]string{"KEY=VAL": "x"},
	})
	if err == nil {
		t.Fatal("expected error for invalid env key")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got: %v", err)
	}
}

func TestDockerRun_InvalidLabelKey(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	_, err := rt.Run(context.Background(), RunRequest{
		ImageURI: "nginx:latest",
		Labels:   map[string]string{"bad=label": "value"},
	})
	if err == nil {
		t.Fatal("expected error for invalid label key")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got: %v", err)
	}
}

func TestDockerRun_CancelledContext(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := rt.Run(ctx, RunRequest{ImageURI: "nginx:latest"})
	// Even with a cancelled context, Run returns a result (may have non-zero exit).
	// It should not return a fatal error; it is retryable or the result is populated.
	if result == nil && err == nil {
		t.Fatal("expected either a result or an error")
	}
}

// ---------------------------------------------------------------------------.
// Start -- always returns ErrMachineGone
// ---------------------------------------------------------------------------.

func TestDockerStart_AlwaysReturnsErrMachineGone(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	err := rt.Start(context.Background(), "any-container", nil)
	if !errors.Is(err, ErrMachineGone) {
		t.Errorf("expected ErrMachineGone, got %v", err)
	}
}

func TestDockerStart_WithEnv_StillReturnsErrMachineGone(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	err := rt.Start(context.Background(), "container-1", map[string]string{"K": "V"})
	if !errors.Is(err, ErrMachineGone) {
		t.Errorf("expected ErrMachineGone, got %v", err)
	}
}

// ---------------------------------------------------------------------------.
// Stop -- error handling when Docker daemon is unavailable
// ---------------------------------------------------------------------------.

func TestDockerStop_NonexistentContainer(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	// Stopping a container that does not exist should return an error.
	err := rt.Stop(context.Background(), "nonexistent-container-stop-test")
	if err == nil {
		t.Fatal("expected error stopping nonexistent container")
	}
}

func TestDockerStop_CancelledContext(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rt.Stop(ctx, "any-container")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// ---------------------------------------------------------------------------.
// Destroy -- error handling
// ---------------------------------------------------------------------------.

func TestDockerDestroy_NonexistentContainer(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	// docker rm -f on a nonexistent container may or may not return an error
	// depending on the Docker version and daemon availability. We verify it
	// does not panic regardless.
	_ = rt.Destroy(context.Background(), "nonexistent-container-destroy-test")
}

func TestDockerDestroy_CancelledContext(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rt.Destroy(ctx, "any-container")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// ---------------------------------------------------------------------------.
// Status -- error handling
// ---------------------------------------------------------------------------.

func TestDockerStatus_NonexistentContainer(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	status, err := rt.Status(context.Background(), "nonexistent-container-status-test")
	if err == nil {
		t.Fatal("expected error for nonexistent container")
	}
	if status != MachineStatusUnknown {
		t.Errorf("status = %v, want %v", status, MachineStatusUnknown)
	}
}

func TestDockerStatus_CancelledContext(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	status, err := rt.Status(ctx, "any-container")
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
	if status != MachineStatusUnknown {
		t.Errorf("status = %v, want %v", status, MachineStatusUnknown)
	}
}

// ---------------------------------------------------------------------------.
// GetLogs -- error handling
// ---------------------------------------------------------------------------.

func TestDockerGetLogs_NonexistentContainer(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	_, err := rt.GetLogs(context.Background(), "nonexistent-container-logs-test", 10)
	if err == nil {
		t.Fatal("expected error for nonexistent container")
	}
}

func TestDockerGetLogs_CancelledContext(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.GetLogs(ctx, "any-container", 10)
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// ---------------------------------------------------------------------------.
// Wait -- timeout handling
// ---------------------------------------------------------------------------.

func TestDockerWait_CancelledContext(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := rt.Wait(ctx, "nonexistent-wait-test", 0)
	// Wait should return a result even on error (exit code 137 for killed).
	if result == nil {
		t.Fatal("expected non-nil result even on cancelled context")
	}
	if result.MachineID != "nonexistent-wait-test" {
		t.Errorf("MachineID = %q, want nonexistent-wait-test", result.MachineID)
	}
	// The error should be nil or a timeout error; the result captures the failure.
	_ = err
}

func TestDockerWait_SetsTimestamps(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	before := time.Now()
	result, _ := rt.Wait(ctx, "nonexistent-timestamps-test", 1)
	after := time.Now()

	if result.StartedAt == nil {
		t.Fatal("expected StartedAt to be set")
	}
	if result.StartedAt.Before(before) || result.StartedAt.After(after) {
		t.Errorf("StartedAt %v not in expected range [%v, %v]", result.StartedAt, before, after)
	}
}

// ---------------------------------------------------------------------------.
// Create/Run -- validation with embedded credentials
// ---------------------------------------------------------------------------.

func TestDockerCreate_EmbeddedCredentialsRejected(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI: "user:pass@registry.example.com/image:v1",
	})
	if err == nil {
		t.Fatal("expected error for embedded credentials in image URI")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got: %v", err)
	}
}

func TestDockerCreate_DigestPinAllowed(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	// This should pass validation (but fail on docker pull -- we just test validation).
	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI: "registry.example.com/image@sha256:abcdef1234567890",
	})
	// Error is expected (docker not available or image not found), but it should
	// NOT be a fatal validation error.
	if err != nil && IsFatal(err) {
		t.Errorf("digest-pinned image should pass validation, got fatal: %v", err)
	}
}

func TestDockerRun_EmbeddedCredentialsRejected(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	_, err := rt.Run(context.Background(), RunRequest{
		ImageURI: "user:pass@registry.example.com/image:v1",
	})
	if err == nil {
		t.Fatal("expected error for embedded credentials in image URI")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------.
// Create -- timeout parameter handling
// ---------------------------------------------------------------------------.

func TestDockerCreate_WithTimeout(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	// We cannot run docker in tests, but we can verify that a positive
	// TimeoutSecs does not cause a panic during argument construction.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.Create(ctx, RunRequest{
		ImageURI:    "nginx:latest",
		TimeoutSecs: 30,
	})
	// Should fail due to cancelled context, not a nil-pointer panic.
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

func TestDockerRun_WithTimeout(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := rt.Run(ctx, RunRequest{
		ImageURI:    "nginx:latest",
		TimeoutSecs: 30,
	})
	// Should produce a result or error, not a panic.
	if result == nil && err == nil {
		t.Fatal("expected either a result or an error")
	}
}

// ---------------------------------------------------------------------------.
// Create/Run -- multiple env vars and labels
// ---------------------------------------------------------------------------.

func TestDockerCreate_MultipleEnvAndLabels(t *testing.T) {
	t.Parallel()
	rt := NewDockerRuntime()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := rt.Create(ctx, RunRequest{
		ImageURI: "nginx:latest",
		Env: map[string]string{
			"KEY_A": "val_a",
			"KEY_B": "val_b",
		},
		Labels: map[string]string{
			"com.example.first":  "one",
			"com.example.second": "two",
		},
	})
	// Fails due to cancelled context, but should not fail on validation.
	if err != nil && IsFatal(err) {
		t.Errorf("valid env/labels should pass validation, got fatal: %v", err)
	}
}
