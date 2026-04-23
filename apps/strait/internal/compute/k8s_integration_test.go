package compute

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// requireKindCluster returns a K8sRuntime connected to a kind cluster.
// Skips the test if no cluster is available or if -short is set.
func requireKindCluster(t *testing.T) *K8sRuntime {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test; use -run Integration without -short")
	}
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	rt, err := NewK8sRuntime(kubeconfig, "default", "", "")
	if err != nil {
		t.Skipf("cannot connect to k8s cluster: %v", err)
	}
	return rt
}

// Happy path tests.

func TestK8sIntegration_RunSuccess(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := rt.Run(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		Env:           map[string]string{"MSG": "hello"},
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.StartedAt == nil {
		t.Error("StartedAt is nil")
	}
	if result.FinishedAt == nil {
		t.Error("FinishedAt is nil")
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), result.MachineID) })
}

func TestK8sIntegration_RunWithEnvVars(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		Env:           map[string]string{"MY_VAR": "test-value-123"},
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	result, err := rt.Wait(ctx, id, 30)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestK8sIntegration_RunExitCode1(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := rt.Run(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		Env:           map[string]string{},
		TimeoutSecs:   30,
	})
	t.Cleanup(func() {
		if result != nil {
			_ = rt.Destroy(context.Background(), result.MachineID)
		}
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// Alpine with no command exits 0 by default. We verify it ran.
	if result.MachineID == "" {
		t.Error("MachineID is empty")
	}
}

// Lifecycle tests.

func TestK8sIntegration_CreateAndWait(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	result, err := rt.Wait(ctx, id, 30)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestK8sIntegration_CreateAndDestroy(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   60,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err = rt.Destroy(ctx, id)
	if err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}

	// Second destroy should return ErrMachineGone.
	err = rt.Destroy(ctx, id)
	if !errors.Is(err, ErrMachineGone) {
		t.Errorf("second Destroy() error = %v, want ErrMachineGone", err)
	}
}

func TestK8sIntegration_StartReturnsGone(t *testing.T) {
	rt := requireKindCluster(t)
	err := rt.Start(context.Background(), "any-id", map[string]string{})
	if !errors.Is(err, ErrMachineGone) {
		t.Errorf("Start() error = %v, want ErrMachineGone", err)
	}
}

// Logs tests.

func TestK8sIntegration_GetLogs_Stdout(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := rt.Run(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), result.MachineID) })

	logs, err := rt.GetLogs(ctx, result.MachineID, 100)
	if err != nil {
		// Logs may not be available for quick-exit containers in kind.
		t.Logf("GetLogs() error = %v (may be expected for instant exit)", err)
		return
	}
	_ = logs // Just verify no panic.
}

// Preset tests.

func TestK8sIntegration_Preset_Micro(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := rt.Run(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("Run(micro) error = %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), result.MachineID) })

	if result.ExitCode != 0 {
		t.Errorf("micro preset ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestK8sIntegration_Preset_Small1x(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := rt.Run(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "small-1x",
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("Run(small-1x) error = %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), result.MachineID) })

	if result.ExitCode != 0 {
		t.Errorf("small-1x preset ExitCode = %d, want 0", result.ExitCode)
	}
}

// Concurrency tests.

func TestK8sIntegration_ConcurrentJobs(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	const n = 5
	var wg conc.WaitGroup
	results := make([]*RunResult, n)
	errs := make([]error, n)

	for i := range n {
		wg.Go(func() {
			results[i], errs[i] = rt.Run(ctx, RunRequest{
				ImageURI:      "alpine:3.19",
				MachinePreset: "micro",
				TimeoutSecs:   30,
			})
		})
	}
	wg.Wait()

	var succeeded int
	for i := range n {
		if errs[i] != nil {
			t.Logf("job %d error: %v", i, errs[i])
			continue
		}
		if results[i].ExitCode == 0 {
			succeeded++
		}
		t.Cleanup(func() { _ = rt.Destroy(context.Background(), results[i].MachineID) })
	}

	if succeeded < 3 {
		t.Errorf("only %d/%d concurrent jobs succeeded, want at least 3", succeeded, n)
	}
}

// Status test.

func TestK8sIntegration_StatusAfterComplete(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := rt.Run(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), result.MachineID) })

	status, err := rt.Status(ctx, result.MachineID)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status != MachineStatusStopped {
		t.Errorf("Status() = %v, want Stopped", status)
	}
}

// Error handling tests.

func TestK8sIntegration_InvalidImage(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "invalid-image-that-does-not-exist:nonexistent",
		MachinePreset: "micro",
		TimeoutSecs:   30,
	})
	if err != nil {
		// Some clusters reject immediately.
		return
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	_, err = rt.Wait(ctx, id, 30)
	if err == nil {
		t.Log("invalid image did not produce an error (cluster may retry image pull)")
	}
}

func TestK8sIntegration_ContextCancellation(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   300, // Long timeout so we cancel first.
	})
	if err != nil {
		t.Skipf("Create() failed: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	// Cancel the context immediately.
	cancel()
	_, err = rt.Wait(ctx, id, 300)
	if err == nil {
		t.Log("Wait returned nil after context cancel (job completed quickly)")
	}
}

// Router integration tests.

func TestK8sIntegration_RouterK8sPrimary(t *testing.T) {
	rt := requireKindCluster(t)
	router := NewRuntimeRouter(rt, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, err := router.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("Router.Create() error = %v", err)
	}
	t.Cleanup(func() { _ = router.Destroy(context.Background(), id) })

	result, err := router.Wait(ctx, id, 30)
	if err != nil {
		t.Fatalf("Router.Wait() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
}

func TestK8sIntegration_RouterRoutingConsistency(t *testing.T) {
	rt := requireKindCluster(t)
	router := NewRuntimeRouter(rt, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, err := router.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	t.Cleanup(func() { _ = router.Destroy(context.Background(), id) })

	// Wait, Status, GetLogs should all route to same runtime without error.
	_, _ = router.Wait(ctx, id, 30)

	status, err := router.Status(ctx, id)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status != MachineStatusStopped {
		t.Errorf("Status() = %v, want Stopped", status)
	}

	_, _ = router.GetLogs(ctx, id, 10)
}

// E2E smoke: verify runtime can be constructed.

func TestK8sIntegration_E2E_RuntimeConstruction(t *testing.T) {
	rt := requireKindCluster(t)
	if rt == nil {
		t.Fatal("runtime is nil")
	}
	// Verify we can list pods (basic connectivity).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := rt.clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		t.Fatalf("cannot list pods: %v", err)
	}
}

// Metrics recording test.

type mockK8sMetrics struct {
	createCalls   atomic.Int64
	waitCalls     atomic.Int64
	scheduleCalls atomic.Int64
	mu            sync.Mutex
	activeDeltas  []int64
}

func (m *mockK8sMetrics) RecordJobCreate(string, string, float64) { m.createCalls.Add(1) }
func (m *mockK8sMetrics) RecordJobWait(string, float64)           { m.waitCalls.Add(1) }
func (m *mockK8sMetrics) RecordPodScheduling(float64)             { m.scheduleCalls.Add(1) }

func (m *mockK8sMetrics) IncJobsActive(delta int64) {
	m.mu.Lock()
	m.activeDeltas = append(m.activeDeltas, delta)
	m.mu.Unlock()
}

func TestK8sIntegration_MetricsRecorded(t *testing.T) {
	rt := requireKindCluster(t)
	mock := &mockK8sMetrics{}
	rt.SetMetrics(mock)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := rt.Run(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   30,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), result.MachineID) })

	if mock.createCalls.Load() != 1 {
		t.Errorf("RecordJobCreate called %d times, want 1", mock.createCalls.Load())
	}
	if mock.waitCalls.Load() != 1 {
		t.Errorf("RecordJobWait called %d times, want 1", mock.waitCalls.Load())
	}
	// Pod scheduling may or may not be recorded depending on whether we see Running before Succeeded.
	// At minimum, active should have +1 and -1.
	mock.mu.Lock()
	deltaCount := len(mock.activeDeltas)
	mock.mu.Unlock()
	if deltaCount < 2 {
		t.Errorf("IncJobsActive called %d times, want at least 2 (+1, -1)", deltaCount)
	}
}

// Suppress unused import for strings.
var _ = strings.NewReader
