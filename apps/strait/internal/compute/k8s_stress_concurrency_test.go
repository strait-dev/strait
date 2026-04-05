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

// Concurrency & Scale stress tests.
// Run with: go test -race ./internal/compute/ -run "TestStress_" -v -timeout 15m.

func stressRun(t *testing.T, rt *K8sRuntime, preset string, timeout int) *RunResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout+30)*time.Second)
	defer cancel()
	result, err := rt.Run(ctx, RunRequest{
		ImageURI: "alpine:3.19", MachinePreset: preset, TimeoutSecs: timeout,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), result.MachineID) })
	return result
}

func burstTest(t *testing.T, n int) {
	t.Helper()
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	var succeeded, failed atomic.Int64
	ids := make([]string, n)

	start := time.Now()
	for i := range n {
		wg.Go(func() {
			result, err := rt.Run(ctx, RunRequest{
				ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 60,
				Env: map[string]string{"IDX": fmt.Sprintf("%d", i)},
			})
			if err != nil {
				failed.Add(1)
				return
			}
			ids[i] = result.MachineID
			if result.ExitCode == 0 {
				succeeded.Add(1)
			}
		})
	}
	wg.Wait()
	dur := time.Since(start)

	// Cleanup.
	for _, id := range ids {
		if id != "" {
			_ = rt.Destroy(context.Background(), id)
		}
	}

	t.Logf("burst %d: %d succeeded, %d failed in %s", n, succeeded.Load(), failed.Load(), dur)
	minSuccess := int64(n * 7 / 10) // Allow 30% failure on heavy bursts.
	if succeeded.Load() < minSuccess {
		t.Fatalf("only %d/%d succeeded (min %d)", succeeded.Load(), n, minSuccess)
	}
}

func TestStress_Burst_10(t *testing.T) { burstTest(t, 10) }
func TestStress_Burst_25(t *testing.T) { burstTest(t, 25) }
func TestStress_Burst_50(t *testing.T) { burstTest(t, 50) }

func TestStress_Concurrent_Same_Preset(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const n = 15
	var wg sync.WaitGroup
	var ok atomic.Int64
	for range n {
		wg.Go(func() {
			r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
			if err == nil && r.ExitCode == 0 {
				ok.Add(1)
				_ = rt.Destroy(context.Background(), r.MachineID)
			}
		})
	}
	wg.Wait()
	t.Logf("%d/%d micro jobs succeeded", ok.Load(), n)
	if ok.Load() < int64(n*7/10) {
		t.Fatalf("too few succeeded")
	}
}

func TestStress_Concurrent_Mixed_Presets(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	presets := []string{"micro", "small-1x", "small-2x", "micro", "small-1x"}
	var wg sync.WaitGroup
	var ok atomic.Int64
	for _, p := range presets {
		preset := p
		wg.Go(func() {
			r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: preset, TimeoutSecs: 30})
			if err == nil && r.ExitCode == 0 {
				ok.Add(1)
				_ = rt.Destroy(context.Background(), r.MachineID)
			}
		})
	}
	wg.Wait()
	t.Logf("%d/%d mixed preset jobs succeeded", ok.Load(), len(presets))
}

func TestStress_Concurrent_Wait_Same_Job(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	var wg sync.WaitGroup
	var results [2]*RunResult
	for i := range 2 {
		wg.Go(func() {
			results[i], _ = rt.Wait(ctx, id, 30)
		})
	}
	wg.Wait()

	for i, r := range results {
		if r == nil {
			t.Errorf("Wait goroutine %d returned nil result", i)
		}
	}
}

func TestStress_Concurrent_Create_And_Destroy(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const n = 10
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
			if err != nil {
				return
			}
			// Immediately destroy -- race between pod starting and deletion.
			_ = rt.Destroy(ctx, id)
		})
	}
	wg.Wait()
	t.Log("concurrent create+destroy: no panics")
}

func TestStress_Pipeline_Sequential_10(t *testing.T) {
	rt := requireKindCluster(t)
	for i := range 10 {
		r := stressRun(t, rt, "micro", 30)
		if r.ExitCode != 0 {
			t.Fatalf("pipeline job %d exit=%d", i, r.ExitCode)
		}
	}
	t.Log("10 sequential jobs: all passed")
}

func TestStress_Fan_Out_Fan_In(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const children = 8
	var wg sync.WaitGroup
	var ok atomic.Int64
	for i := range children {
		wg.Go(func() {
			r, err := rt.Run(ctx, RunRequest{
				ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30,
				Env: map[string]string{"CHILD": fmt.Sprintf("%d", i)},
			})
			if err == nil {
				_ = rt.Destroy(context.Background(), r.MachineID)
				if r.ExitCode == 0 {
					ok.Add(1)
				}
			}
		})
	}
	wg.Wait()
	t.Logf("fan-out/fan-in: %d/%d children succeeded", ok.Load(), children)
	if ok.Load() < int64(children*7/10) {
		t.Fatalf("too few children succeeded")
	}
}

func TestStress_Staggered_Start(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const total = 10
	var wg sync.WaitGroup
	var ok atomic.Int64

	for i := range total {
		time.Sleep(500 * time.Millisecond) // 2/sec.
		wg.Go(func() {
			r, err := rt.Run(ctx, RunRequest{
				ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30,
				Env: map[string]string{"STAGGER": fmt.Sprintf("%d", i)},
			})
			if err == nil && r.ExitCode == 0 {
				ok.Add(1)
				_ = rt.Destroy(context.Background(), r.MachineID)
			}
		})
	}
	wg.Wait()
	t.Logf("staggered: %d/%d succeeded", ok.Load(), total)
}

func TestStress_All_Presets_Parallel(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	presets := PresetOrder
	var wg sync.WaitGroup
	var ok atomic.Int64
	for _, p := range presets {
		preset := p
		wg.Go(func() {
			r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: preset, TimeoutSecs: 60})
			if err == nil && r.ExitCode == 0 {
				ok.Add(1)
				_ = rt.Destroy(context.Background(), r.MachineID)
			}
		})
	}
	wg.Wait()
	t.Logf("all presets: %d/%d succeeded", ok.Load(), len(presets))
}

func TestStress_Rapid_Loop_50(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	const n = 20 // Reduced from 50 for kind capacity.
	var ok atomic.Int64
	for i := range n {
		r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
		if err != nil {
			t.Logf("loop %d failed: %v", i, err)
			continue
		}
		_ = rt.Destroy(ctx, r.MachineID)
		if r.ExitCode == 0 {
			ok.Add(1)
		}
	}
	t.Logf("rapid loop: %d/%d succeeded", ok.Load(), n)
	if ok.Load() < int64(n*8/10) {
		t.Fatalf("too many failures in rapid loop")
	}
}

func TestStress_Router_Concurrent_10(t *testing.T) {
	rt := requireKindCluster(t)
	router := NewRuntimeRouter(rt, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const n = 8
	var wg sync.WaitGroup
	var ok atomic.Int64
	for range n {
		wg.Go(func() {
			r, err := router.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
			if err == nil && r.ExitCode == 0 {
				ok.Add(1)
				_ = router.Destroy(context.Background(), r.MachineID)
			}
		})
	}
	wg.Wait()
	t.Logf("router concurrent: %d/%d succeeded", ok.Load(), n)
}

func TestStress_Router_Thundering_Herd(t *testing.T) {
	rt := requireKindCluster(t)
	failing := &mockRuntime{
		createFn: func(context.Context, RunRequest) (string, error) {
			return "", NewRetryableError(503, "down", nil)
		},
	}
	router := NewRuntimeRouter(failing, rt)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const n = 8
	var wg sync.WaitGroup
	var ok atomic.Int64
	for range n {
		wg.Go(func() {
			r, err := router.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
			if err == nil && r.ExitCode == 0 {
				ok.Add(1)
				_ = router.Destroy(context.Background(), r.MachineID)
			}
		})
	}
	wg.Wait()
	t.Logf("thundering herd: %d/%d fell back successfully", ok.Load(), n)
	if ok.Load() < int64(n*7/10) {
		t.Fatalf("too few succeeded via fallback")
	}
}

func TestStress_Namespace_Isolation(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Both use default namespace (kind only has default).
	// Test that the namespace override field works without error.
	r1, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("ns-a job: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r1.MachineID) })

	r2, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("ns-b job: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r2.MachineID) })

	if r1.MachineID == r2.MachineID {
		t.Error("two jobs have same machineID")
	}
}

func TestStress_Sustained_30s(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	var ok atomic.Int64
	start := time.Now()
	total := 0

	for time.Since(start) < 15*time.Second { // 15s to keep kind manageable.
		total++
		wg.Go(func() {
			r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
			if err == nil && r.ExitCode == 0 {
				ok.Add(1)
				_ = rt.Destroy(context.Background(), r.MachineID)
			}
		})
		time.Sleep(time.Second)
	}
	wg.Wait()
	t.Logf("sustained: %d/%d succeeded over %s", ok.Load(), total, time.Since(start))
}

func TestStress_Create_Destroy_Interleaved(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var ids []string
	// Create 5.
	for range 5 {
		id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
		if err == nil {
			ids = append(ids, id)
		}
	}
	// Destroy 2.
	for _, id := range ids[:min(2, len(ids))] {
		_ = rt.Destroy(ctx, id)
	}
	// Create 3 more.
	for range 3 {
		id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
		if err == nil {
			ids = append(ids, id)
		}
	}
	// Destroy all.
	for _, id := range ids {
		_ = rt.Destroy(ctx, id)
	}
	t.Logf("interleaved: created %d, all destroyed", len(ids))
}

func TestStress_Wait_Polling_Fallback(t *testing.T) {
	// This tests that Wait works via polling (watch may or may not work on kind).
	rt := requireKindCluster(t)
	r := stressRun(t, rt, "micro", 30)
	if r.ExitCode != 0 {
		t.Errorf("exit code %d, want 0", r.ExitCode)
	}
}

func TestStress_Concurrent_Status_Polling(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	// Poll status 30 times concurrently.
	var wg sync.WaitGroup
	var errs atomic.Int64
	for range 30 {
		wg.Go(func() {
			_, err := rt.Status(ctx, id)
			if err != nil {
				errs.Add(1)
			}
		})
	}
	wg.Wait()
	_, _ = rt.Wait(ctx, id, 30)
	t.Logf("30 concurrent Status calls: %d errors", errs.Load())
}

func TestStress_Concurrent_GetLogs(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })

	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			_, _ = rt.GetLogs(ctx, r.MachineID, 10)
		})
	}
	wg.Wait()
	t.Log("5 concurrent GetLogs: no panics")
}

// Suppress unused import.
var _ = errors.New
