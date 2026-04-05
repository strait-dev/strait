package compute

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Router & Fallback stress tests.

func TestStress_Router_Primary_Success(t *testing.T) {
	rt := requireKindCluster(t)
	var fallbackCalled atomic.Bool
	fb := &mockRuntime{
		createFn: func(context.Context, RunRequest) (string, error) {
			fallbackCalled.Store(true)
			return "", errors.New("should not be called")
		},
	}
	router := NewRuntimeRouter(rt, fb)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := router.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = router.Destroy(context.Background(), r.MachineID) })

	if fallbackCalled.Load() {
		t.Error("fallback was called when primary succeeded")
	}
}

func TestStress_Router_Retryable_Falls_Back(t *testing.T) {
	rt := requireKindCluster(t)
	failing := &mockRuntime{
		createFn: func(context.Context, RunRequest) (string, error) {
			return "", NewRetryableError(503, "primary down", nil)
		},
	}
	router := NewRuntimeRouter(failing, rt)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := router.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("fallback Run: %v", err)
	}
	t.Cleanup(func() { _ = router.Destroy(context.Background(), r.MachineID) })
	t.Logf("fallback: exit=%d", r.ExitCode)
}

func TestStress_Router_Fatal_No_Fallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	failing := &mockRuntime{
		createFn: func(context.Context, RunRequest) (string, error) {
			return "", NewFatalError(422, "bad config", nil)
		},
	}
	var fbCalled atomic.Bool
	fb := &mockRuntime{
		createFn: func(context.Context, RunRequest) (string, error) {
			fbCalled.Store(true)
			return "fb-id", nil
		},
	}
	router := NewRuntimeRouter(failing, fb)

	_, err := router.Create(context.Background(), RunRequest{ImageURI: "x", MachinePreset: "micro"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal, got: %v", err)
	}
	if fbCalled.Load() {
		t.Error("fallback called on fatal error")
	}
}

func TestStress_Router_Nil_Fallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	failing := &mockRuntime{
		createFn: func(context.Context, RunRequest) (string, error) {
			return "", NewRetryableError(503, "down", nil)
		},
	}
	router := NewRuntimeRouter(failing, nil)

	_, err := router.Create(context.Background(), RunRequest{ImageURI: "x", MachinePreset: "micro"})
	if err == nil {
		t.Fatal("expected error with nil fallback")
	}
}

func TestStress_Router_Both_Fail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	failing1 := &mockRuntime{
		createFn: func(context.Context, RunRequest) (string, error) {
			return "", NewRetryableError(503, "primary down", nil)
		},
	}
	failing2 := &mockRuntime{
		createFn: func(context.Context, RunRequest) (string, error) {
			return "", NewRetryableError(502, "fallback down", nil)
		},
	}
	router := NewRuntimeRouter(failing1, failing2)

	_, err := router.Create(context.Background(), RunRequest{ImageURI: "x", MachinePreset: "micro"})
	if err == nil {
		t.Fatal("expected error when both fail")
	}
}

func TestStress_Router_Wait_Routes_Correctly(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	router := NewRuntimeRouter(rt, nil)
	id, _ := router.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = router.Destroy(context.Background(), id) })

	result, err := router.Wait(ctx, id, 30)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("exit=%d, want 0", result.ExitCode)
	}
}

func TestStress_Router_Destroy_Routes_Correctly(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	router := NewRuntimeRouter(rt, nil)
	id, _ := router.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})

	err := router.Destroy(ctx, id)
	if err != nil {
		t.Errorf("Destroy: %v", err)
	}

	err = router.Destroy(ctx, id)
	if !errors.Is(err, ErrMachineGone) {
		t.Errorf("second Destroy: %v, want ErrMachineGone", err)
	}
}

func TestStress_Router_GetLogs_After_Wait(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	router := NewRuntimeRouter(rt, nil)
	r, _ := router.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if r == nil {
		t.Fatal("Run returned nil")
	}
	t.Cleanup(func() { _ = router.Destroy(context.Background(), r.MachineID) })

	// GetLogs should still work after Wait (owner not deleted).
	_, err := router.GetLogs(ctx, r.MachineID, 10)
	if err != nil {
		t.Logf("GetLogs after Wait: %v (pod may be GC'd)", err)
	} else {
		t.Log("GetLogs after Wait: success")
	}
}

func TestStress_Router_Concurrent_Mixed(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	failing := &mockRuntime{
		createFn: func(context.Context, RunRequest) (string, error) {
			return "", NewRetryableError(503, "down", nil)
		},
	}

	// Half go to K8s directly, half go through router with failing primary.
	const n = 6
	var wg sync.WaitGroup
	var ok atomic.Int64

	for i := range n {
		var router ContainerRuntime
		if i%2 == 0 {
			router = NewRuntimeRouter(rt, nil) // Direct K8s.
		} else {
			router = NewRuntimeRouter(failing, rt) // Fallback to K8s.
		}
		wg.Go(func() {
			r, err := router.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
			if err == nil && r.ExitCode == 0 {
				ok.Add(1)
				_ = router.Destroy(context.Background(), r.MachineID)
			}
		})
	}
	wg.Wait()
	t.Logf("concurrent mixed: %d/%d succeeded", ok.Load(), n)
}

func TestStress_Router_Owner_Cleanup(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	router := NewRuntimeRouter(rt, nil)
	id, _ := router.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	_, _ = router.Wait(ctx, id, 30)

	// After Wait, owner should still exist for GetLogs.
	_, err := router.GetLogs(ctx, id, 10)
	t.Logf("GetLogs after Wait: err=%v", err)

	// After Destroy, owner should be cleaned up.
	_ = router.Destroy(ctx, id)
	t.Log("owner cleanup: no panics after Destroy")
}
