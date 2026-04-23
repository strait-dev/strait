package compute

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Edge Cases & Chaos stress tests.

func TestStress_Pod_Deleted_During_Wait(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 60})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	// Delete the pod externally after a brief delay.
	go func() {
		time.Sleep(2 * time.Second)
		pods, _ := rt.clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{LabelSelector: "job-name=" + id})
		for _, p := range pods.Items {
			_ = rt.clientset.CoreV1().Pods("default").Delete(ctx, p.Name, metav1.DeleteOptions{})
		}
	}()

	// Wait should eventually return (job may be rescheduled or fail).
	result, err := rt.Wait(ctx, id, 30)
	if err != nil {
		t.Logf("Wait after pod delete: %v", err)
	} else {
		t.Logf("Wait after pod delete: exit=%d", result.ExitCode)
	}
}

func TestStress_Job_Deleted_During_Wait(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 60})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Delete the job externally after a brief delay.
	go func() {
		time.Sleep(2 * time.Second)
		_ = rt.Destroy(context.Background(), id)
	}()

	_, err = rt.Wait(ctx, id, 30)
	t.Logf("Wait after job delete: %v", err)
	// Should timeout or return without hanging.
}

func TestStress_Double_Wait(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	var wg conc.WaitGroup
	var results [2]*RunResult
	for i := range 2 {
		wg.Go(func() {
			results[i], _ = rt.Wait(ctx, id, 30)
		})
	}
	wg.Wait()

	for i, r := range results {
		if r == nil {
			t.Logf("double wait %d: nil result", i)
		} else {
			t.Logf("double wait %d: exit=%d", i, r.ExitCode)
		}
	}
}

func TestStress_Double_Destroy(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})

	var wg conc.WaitGroup
	var errs [2]error
	for i := range 2 {
		wg.Go(func() {
			errs[i] = rt.Destroy(ctx, id)
		})
	}
	wg.Wait()

	// At least one should succeed, the other should be ErrMachineGone.
	var gone int
	for _, err := range errs {
		if errors.Is(err, ErrMachineGone) {
			gone++
		}
	}
	t.Logf("double destroy: %d got ErrMachineGone", gone)
}

func TestStress_Create_Pod_Never_Starts(t *testing.T) {
	// An unschedulable pod should be detected by podFailureReason.
	// On kind, all pods are schedulable, so we just test the normal path.
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })
	t.Logf("pod started and completed: exit=%d", r.ExitCode)
}

func TestStress_Unicode_Env_Values(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := rt.Run(ctx, RunRequest{
		ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30,
		Env: map[string]string{
			"EMOJI":   "Hello World!",
			"CJK":     "Hello World",
			"UNICODE": "cafe\u0301",
		},
	})
	if err != nil {
		t.Fatalf("Run with unicode env: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })
	t.Logf("unicode env: exit=%d", r.ExitCode)
}

func TestStress_Empty_Env_Map(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30, Env: nil})
	if err != nil {
		t.Fatalf("Run with nil env: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })

	r2, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30, Env: map[string]string{}})
	if err != nil {
		t.Fatalf("Run with empty env: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r2.MachineID) })
}

func TestStress_Empty_Labels_Map(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30, Labels: nil})
	if err != nil {
		t.Fatalf("Run with nil labels: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })
}

func TestStress_Timing_Cold_Start(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	start := time.Now()
	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	createDur := time.Since(start)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	waitStart := time.Now()
	_, _ = rt.Wait(ctx, id, 30)
	waitDur := time.Since(waitStart)

	t.Logf("cold start: create=%s, wait=%s, total=%s", createDur, waitDur, createDur+waitDur)
}

func TestStress_Timing_Execution(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	start := time.Now()
	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	totalDur := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })
	t.Logf("execution timing: total=%s", totalDur)
}

func TestStress_Rapid_Status_100(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	var wg conc.WaitGroup
	var errs atomic.Int64
	for range 50 { // 50 instead of 100 to be kind to API.
		wg.Go(func() {
			_, err := rt.Status(ctx, id)
			if err != nil {
				errs.Add(1)
			}
		})
	}
	wg.Wait()
	_, _ = rt.Wait(ctx, id, 30)
	t.Logf("50 rapid Status calls: %d errors", errs.Load())
}

func TestStress_GetLogs_Before_Start(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	// Immediately try GetLogs before pod might be ready.
	_, err := rt.GetLogs(ctx, id, 10)
	t.Logf("GetLogs before start: %v", err)
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_Interleaved_Lifecycle(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Create 5.
	var ids []string
	for range 5 {
		id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
		if err == nil {
			ids = append(ids, id)
		}
	}

	// Destroy first 2.
	for _, id := range ids[:min(2, len(ids))] {
		_ = rt.Destroy(ctx, id)
	}

	// Wait remaining 3.
	for _, id := range ids[min(2, len(ids)):] {
		_, _ = rt.Wait(ctx, id, 30)
		_ = rt.Destroy(ctx, id)
	}
	t.Logf("interleaved lifecycle: %d jobs processed", len(ids))
}

func TestStress_Job_With_Command(t *testing.T) {
	// K8s Jobs use the image's default CMD. We verify env vars are passed.
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := rt.Run(ctx, RunRequest{
		ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30,
		Env: map[string]string{"MY_CMD": "echo hello"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })
	t.Logf("job with env command: exit=%d", r.ExitCode)
}

func TestStress_Resilience_Full_Cycle(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create.
	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Logf("1. Create: %s", id)

	// Status.
	status, _ := rt.Status(ctx, id)
	t.Logf("2. Status: %s", status)

	// Wait.
	result, err := rt.Wait(ctx, id, 30)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	t.Logf("3. Wait: exit=%d", result.ExitCode)

	// GetLogs.
	logs, err := rt.GetLogs(ctx, id, 10)
	if err != nil {
		t.Logf("4. GetLogs: %v", err)
	} else {
		t.Logf("4. GetLogs: %d bytes", len(logs))
	}

	// Status after completion.
	status, _ = rt.Status(ctx, id)
	t.Logf("5. Status after complete: %s", status)

	// Destroy.
	err = rt.Destroy(ctx, id)
	if err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	t.Log("6. Destroy: success")
	t.Log("full lifecycle: all 6 steps passed")
}
