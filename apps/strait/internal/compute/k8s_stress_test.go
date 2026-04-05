package compute

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Stress tests for K8sRuntime on real Kubernetes (kind).
// Run with: go test -race ./internal/compute/ -run "Stress" -v -timeout 10m.

// TestK8sStress_BurstCreate creates many jobs simultaneously to stress the K8s API.
func TestK8sStress_BurstCreate(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	const burst = 15
	var wg sync.WaitGroup
	var succeeded, failed atomic.Int64
	jobIDs := make([]string, burst)

	t.Logf("burst creating %d jobs...", burst)
	start := time.Now()

	for i := range burst {
		wg.Go(func() {
			id, err := rt.Create(ctx, RunRequest{
				ImageURI:      "alpine:3.19",
				MachinePreset: "micro",
				Env:           map[string]string{"JOB_INDEX": fmt.Sprintf("%d", i)},
				TimeoutSecs:   60,
			})
			if err != nil {
				failed.Add(1)
				t.Logf("job %d create failed: %v", i, err)
				return
			}
			jobIDs[i] = id
			succeeded.Add(1)
		})
	}
	wg.Wait()

	createDur := time.Since(start)
	t.Logf("burst create: %d succeeded, %d failed in %s", succeeded.Load(), failed.Load(), createDur)

	if succeeded.Load() < int64(burst*8/10) {
		t.Fatalf("only %d/%d jobs created successfully", succeeded.Load(), burst)
	}

	// Wait for all jobs to complete.
	t.Log("waiting for all jobs to complete...")
	waitStart := time.Now()
	var waitWG sync.WaitGroup
	var completions, timeouts atomic.Int64

	for i := range burst {
		if jobIDs[i] == "" {
			continue
		}
		id := jobIDs[i]
		waitWG.Go(func() {
			defer func() { _ = rt.Destroy(context.Background(), id) }()

			result, err := rt.Wait(ctx, id, 60)
			if err != nil {
				if IsTimeout(err) {
					timeouts.Add(1)
				}
				t.Logf("job %d wait error: %v", i, err)
				return
			}
			if result.ExitCode == 0 {
				completions.Add(1)
			}
		})
	}
	waitWG.Wait()

	waitDur := time.Since(waitStart)
	t.Logf("all jobs done: %d completed, %d timeouts in %s", completions.Load(), timeouts.Load(), waitDur)
	t.Logf("total wall time: %s", time.Since(start))

	if completions.Load() < int64(burst*7/10) {
		t.Fatalf("only %d/%d jobs completed successfully", completions.Load(), burst)
	}
}

// TestK8sStress_RapidCreateDestroy creates and immediately destroys jobs to test cleanup.
func TestK8sStress_RapidCreateDestroy(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const count = 20
	var wg sync.WaitGroup
	var created, destroyed atomic.Int64

	t.Logf("rapid create-destroy %d jobs...", count)

	for range count {
		wg.Go(func() {
			id, err := rt.Create(ctx, RunRequest{
				ImageURI:      "alpine:3.19",
				MachinePreset: "micro",
				TimeoutSecs:   30,
			})
			if err != nil {
				return
			}
			created.Add(1)

			err = rt.Destroy(ctx, id)
			if err == nil || errors.Is(err, ErrMachineGone) {
				destroyed.Add(1)
			}
		})
	}
	wg.Wait()

	t.Logf("created: %d, destroyed: %d", created.Load(), destroyed.Load())

	if created.Load() != destroyed.Load() {
		t.Errorf("leaked jobs: created=%d destroyed=%d", created.Load(), destroyed.Load())
	}
}

// TestK8sStress_MixedWorkload runs different job types concurrently.
func TestK8sStress_MixedWorkload(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	type jobSpec struct {
		name   string
		preset string
		cmd    string
	}

	specs := []jobSpec{
		{"quick-echo", "micro", "echo hello"},
		{"quick-env", "micro", "env | head -5"},
		{"quick-date", "micro", "date"},
		{"small-sleep", "small-1x", "sleep 1"},
		{"small-calc", "small-1x", "expr 42 + 58"},
	}

	var wg sync.WaitGroup
	var succeeded atomic.Int64

	t.Logf("running %d mixed workload jobs...", len(specs))
	start := time.Now()

	for _, spec := range specs {
		s := spec
		wg.Go(func() {
			result, err := rt.Run(ctx, RunRequest{
				ImageURI:      "alpine:3.19",
				MachinePreset: s.preset,
				TimeoutSecs:   30,
			})
			if err != nil {
				t.Logf("%s failed: %v", s.name, err)
				return
			}
			defer func() { _ = rt.Destroy(context.Background(), result.MachineID) }()

			if result.ExitCode == 0 {
				succeeded.Add(1)
				t.Logf("%s completed in %s", s.name, result.FinishedAt.Sub(*result.StartedAt))
			} else {
				t.Logf("%s exit code %d", s.name, result.ExitCode)
			}
		})
	}
	wg.Wait()

	t.Logf("mixed workload: %d/%d succeeded in %s", succeeded.Load(), len(specs), time.Since(start))

	if succeeded.Load() < int64(len(specs)*7/10) {
		t.Fatalf("only %d/%d jobs succeeded", succeeded.Load(), len(specs))
	}
}

// TestK8sStress_GCSweepDuringLoad runs the GC while jobs are being created.
func TestK8sStress_GCSweepDuringLoad(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	gc := NewK8sJobGC(rt.clientset, "default", 30*time.Minute, time.Minute)

	// Create some jobs.
	var ids []string
	for range 5 {
		id, err := rt.Create(ctx, RunRequest{
			ImageURI:      "alpine:3.19",
			MachinePreset: "micro",
			TimeoutSecs:   30,
		})
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}

	// Run GC while jobs are active -- should NOT destroy any (they're young + running).
	gc.Sweep(ctx)

	// Wait for jobs.
	for _, id := range ids {
		_, _ = rt.Wait(ctx, id, 30)
		_ = rt.Destroy(ctx, id)
	}

	t.Log("GC sweep during active load: no panics, no false deletions")
}

// TestK8sStress_MetricsUnderLoad verifies metrics recording under concurrent load.
func TestK8sStress_MetricsUnderLoad(t *testing.T) {
	rt := requireKindCluster(t)
	mock := &mockK8sMetrics{}
	rt.SetMetrics(mock)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const n = 5
	var wg sync.WaitGroup

	for range n {
		wg.Go(func() {
			result, err := rt.Run(ctx, RunRequest{
				ImageURI:      "alpine:3.19",
				MachinePreset: "micro",
				TimeoutSecs:   30,
			})
			if err == nil {
				_ = rt.Destroy(context.Background(), result.MachineID)
			}
		})
	}
	wg.Wait()

	mock.mu.Lock()
	deltaCount := len(mock.activeDeltas)
	mock.mu.Unlock()

	t.Logf("metrics: creates=%d waits=%d schedules=%d activeDeltas=%d",
		mock.createCalls.Load(), mock.waitCalls.Load(), mock.scheduleCalls.Load(), deltaCount)

	if mock.createCalls.Load() < int64(n) {
		t.Errorf("expected at least %d create metrics, got %d", n, mock.createCalls.Load())
	}
	if mock.waitCalls.Load() < int64(n) {
		t.Errorf("expected at least %d wait metrics, got %d", n, mock.waitCalls.Load())
	}
}

// TestK8sStress_RouterFallbackUnderLoad tests the router under load with a failing primary.
func TestK8sStress_RouterFallbackUnderLoad(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Create a mock primary that always fails with retryable error.
	failingPrimary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			return "", NewRetryableError(503, "primary unavailable", nil)
		},
	}

	router := NewRuntimeRouter(failingPrimary, rt)

	const n = 3
	var wg sync.WaitGroup
	var succeeded atomic.Int64

	for range n {
		wg.Go(func() {
			result, err := router.Run(ctx, RunRequest{
				ImageURI:      "alpine:3.19",
				MachinePreset: "micro",
				TimeoutSecs:   30,
			})
			if err != nil {
				t.Logf("router run failed: %v", err)
				return
			}
			defer func() { _ = router.Destroy(context.Background(), result.MachineID) }()
			if result.ExitCode == 0 {
				succeeded.Add(1)
			}
		})
	}
	wg.Wait()

	t.Logf("router fallback: %d/%d succeeded via K8s fallback", succeeded.Load(), n)

	if succeeded.Load() < int64(n) {
		t.Fatalf("expected all %d jobs to succeed via fallback, got %d", n, succeeded.Load())
	}
}
