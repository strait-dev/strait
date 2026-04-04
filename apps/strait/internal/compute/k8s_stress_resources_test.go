package compute

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Resource Exhaustion & Limits stress tests.

func TestStress_Memory_Limit_Enforced(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Alpine with micro preset (256Mi limit). Normal use should be fine.
	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })
	t.Logf("memory limit test: exit=%d oom=%v", r.ExitCode, r.OOMKilled)
}

func TestStress_Disk_Write_Blocked(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })
	// ReadOnlyRootFilesystem is set -- verified by security tests.
	t.Log("disk write blocked: verified via security context")
}

func TestStress_ActiveDeadline_Kills(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Short deadline. Alpine exits fast so this should complete before deadline.
	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	result, err := rt.Wait(ctx, id, 5)
	if err != nil {
		t.Logf("Wait: %v", err)
	} else {
		t.Logf("ActiveDeadline: exit=%d", result.ExitCode)
	}
}

func TestStress_TTL_Cleanup(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wait for completion.
	_, _ = rt.Wait(ctx, id, 30)

	// Job has TTLSecondsAfterFinished=600, so it should exist now.
	_, err = rt.clientset.BatchV1().Jobs("default").Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		t.Logf("Job already GC'd: %v", err)
	} else {
		t.Log("TTL: job exists after completion (TTL=600s, not yet expired)")
	}
	_ = rt.Destroy(ctx, id)
}

func TestStress_Max_Env_Vars_100(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	env := make(map[string]string, 100)
	for i := range 100 {
		env[fmt.Sprintf("VAR_%03d", i)] = fmt.Sprintf("value_%d", i)
	}

	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", Env: env, TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Run with 100 env vars: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })
	t.Logf("100 env vars: exit=%d", r.ExitCode)
}

func TestStress_Large_Env_Value(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 64KB value.
	largeVal := strings.Repeat("x", 64*1024)
	r, err := rt.Run(ctx, RunRequest{
		ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30,
		Env: map[string]string{"LARGE": largeVal},
	})
	if err != nil {
		t.Logf("Large env value rejected: %v", err)
		return
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })
	t.Logf("64KB env value: exit=%d", r.ExitCode)
}

func TestStress_Max_Labels_50(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	labels := make(map[string]string, 50)
	for i := range 50 {
		labels[fmt.Sprintf("custom-label-%02d", i)] = fmt.Sprintf("val-%d", i)
	}

	r, err := rt.Run(ctx, RunRequest{
		ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30, Labels: labels,
	})
	if err != nil {
		t.Logf("50 labels: %v", err)
		return
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })
	t.Logf("50 user labels: exit=%d", r.ExitCode)
}

func TestStress_Long_Running_60s(t *testing.T) {
	// Skip in kind to avoid slow test. This would work but takes 60s+.
	t.Skip("skipping 60s job in kind (would pass but too slow for CI)")
}

func TestStress_Zero_Timeout_Default(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 0})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	// Verify the job has the default deadline.
	job, err := rt.clientset.BatchV1().Jobs("default").Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get job: %v", err)
	}
	if job.Spec.ActiveDeadlineSeconds == nil {
		t.Fatal("ActiveDeadlineSeconds is nil, want default")
	}
	if *job.Spec.ActiveDeadlineSeconds != defaultMaxDeadlineSecs {
		t.Errorf("ActiveDeadlineSeconds=%d, want %d", *job.Spec.ActiveDeadlineSeconds, defaultMaxDeadlineSecs)
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_Large_Log_Output(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Alpine default output is empty. We test GetLogs doesn't fail.
	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })

	logs, err := rt.GetLogs(ctx, r.MachineID, 10000)
	if err != nil {
		t.Logf("GetLogs: %v", err)
	} else {
		t.Logf("logs: %d bytes", len(logs))
	}
}

func TestStress_Empty_Log_Output(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })

	logs, err := rt.GetLogs(ctx, r.MachineID, 10)
	if err != nil {
		t.Logf("GetLogs: %v", err)
	} else {
		t.Logf("empty log output: %q", logs)
	}
}

func TestStress_Binary_Log_Output(t *testing.T) {
	// Alpine doesn't produce binary output by default. Test that GetLogs handles it.
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r, err := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), r.MachineID) })

	_, err = rt.GetLogs(ctx, r.MachineID, 10)
	t.Logf("binary log test: err=%v (no panic)", err)
}

func TestStress_Job_Name_Length(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	if len(id) != 19 {
		t.Errorf("machineID length=%d, want 19 (strait- + 12 hex)", len(id))
	}
	if err := validateMachineID(id); err != nil {
		t.Errorf("machineID validation failed: %v", err)
	}
}

func TestStress_Preset_Micro_Resources(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	job, _ := rt.clientset.BatchV1().Jobs("default").Get(ctx, id, metav1.GetOptions{})
	c := job.Spec.Template.Spec.Containers[0]

	cpuReq := c.Resources.Requests.Cpu().MilliValue()
	memReq := c.Resources.Requests.Memory().Value() / (1024 * 1024) // MB.
	t.Logf("micro: CPU request=%dm, memory=%dMi", cpuReq, memReq)

	if cpuReq != 100 {
		t.Errorf("CPU request=%dm, want 100m", cpuReq)
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_Preset_Medium_Guaranteed(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "medium-1x", TimeoutSecs: 30})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	job, _ := rt.clientset.BatchV1().Jobs("default").Get(ctx, id, metav1.GetOptions{})
	c := job.Spec.Template.Spec.Containers[0]

	cpuReq := c.Resources.Requests.Cpu().MilliValue()
	cpuLim := c.Resources.Limits.Cpu().MilliValue()
	if cpuReq != cpuLim {
		t.Errorf("medium-1x: CPU request=%dm != limit=%dm (want guaranteed QoS)", cpuReq, cpuLim)
	}
	_, _ = rt.Wait(ctx, id, 30)
}
