package compute

import (
	"context"
	"errors"
	"sync"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func newTestK8sRuntime() (*K8sRuntime, *fake.Clientset) {
	cs := fake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "test-ns", "strait-job")
	return rt, cs
}

func TestK8sRuntime_Create_JobSpec(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "node:20-alpine",
		MachinePreset: "small-1x",
		Env:           map[string]string{"FOO": "bar", "BAZ": "qux"},
		Labels:        map[string]string{"project": "test-project"},
		TimeoutSecs:   120,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if id == "" {
		t.Fatal("Create() returned empty machineID")
	}

	job, err := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get job: %v", err)
	}

	// Verify basic spec.
	if job.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("RestartPolicy = %v, want Never", job.Spec.Template.Spec.RestartPolicy)
	}
	if *job.Spec.BackoffLimit != 0 {
		t.Errorf("BackoffLimit = %d, want 0", *job.Spec.BackoffLimit)
	}
	if job.Spec.Template.Spec.PriorityClassName != "strait-job" {
		t.Errorf("PriorityClassName = %q, want strait-job", job.Spec.Template.Spec.PriorityClassName)
	}

	// Verify container.
	if len(job.Spec.Template.Spec.Containers) != 1 {
		t.Fatalf("got %d containers, want 1", len(job.Spec.Template.Spec.Containers))
	}
	c := job.Spec.Template.Spec.Containers[0]
	if c.Image != "node:20-alpine" {
		t.Errorf("Image = %q, want node:20-alpine", c.Image)
	}
	if c.Name != "job" {
		t.Errorf("Name = %q, want job", c.Name)
	}

	// Verify env vars.
	envMap := make(map[string]string)
	for _, e := range c.Env {
		envMap[e.Name] = e.Value
	}
	if envMap["FOO"] != "bar" {
		t.Errorf("env FOO = %q, want bar", envMap["FOO"])
	}
	if envMap["BAZ"] != "qux" {
		t.Errorf("env BAZ = %q, want qux", envMap["BAZ"])
	}

	// Verify timeout.
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != 120 {
		t.Errorf("ActiveDeadlineSeconds = %v, want 120", job.Spec.ActiveDeadlineSeconds)
	}

	// Verify labels.
	if job.Labels["project"] != "test-project" {
		t.Errorf("Labels[project] = %q, want test-project", job.Labels["project"])
	}
	if job.Labels["app"] != "strait-job" {
		t.Errorf("Labels[app] = %q, want strait-job", job.Labels["app"])
	}
}

func TestK8sRuntime_Create_PresetMapping_AllPresets(t *testing.T) {
	tests := []struct {
		preset    string
		wantBurst bool // CPU request < limit (burstable)
	}{
		{"micro", true},
		{"small-1x", true},
		{"small-2x", true},
		{"medium-1x", false}, // guaranteed
		{"medium-2x", false},
		{"large-1x", false},
		{"large-2x", false},
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			rt, cs := newTestK8sRuntime()
			ctx := context.Background()

			id, err := rt.Create(ctx, RunRequest{
				ImageURI:      "alpine:3.19",
				MachinePreset: tt.preset,
			})
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			job, _ := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
			c := job.Spec.Template.Spec.Containers[0]

			cpuReq := c.Resources.Requests[corev1.ResourceCPU]
			cpuLim := c.Resources.Limits[corev1.ResourceCPU]
			memReq := c.Resources.Requests[corev1.ResourceMemory]
			memLim := c.Resources.Limits[corev1.ResourceMemory]

			// Memory request always equals limit.
			if memReq.Cmp(memLim) != 0 {
				t.Errorf("memory request (%s) != limit (%s)", memReq.String(), memLim.String())
			}

			if tt.wantBurst {
				if cpuReq.Cmp(cpuLim) >= 0 {
					t.Errorf("burstable preset %s: CPU request (%s) should be < limit (%s)", tt.preset, cpuReq.String(), cpuLim.String())
				}
			} else {
				if cpuReq.Cmp(cpuLim) != 0 {
					t.Errorf("guaranteed preset %s: CPU request (%s) should equal limit (%s)", tt.preset, cpuReq.String(), cpuLim.String())
				}
			}

			// Verify non-zero resources.
			if cpuReq.IsZero() || memReq.IsZero() {
				t.Errorf("preset %s has zero resources: cpu=%s mem=%s", tt.preset, cpuReq.String(), memReq.String())
			}
		})
	}
}

func TestK8sRuntime_Create_InvalidPreset(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for invalid preset")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got retryable: %v", err)
	}
}

func TestK8sRuntime_Create_NoTimeout(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   0,
	})

	job, _ := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	if job.Spec.ActiveDeadlineSeconds != nil {
		t.Errorf("ActiveDeadlineSeconds = %v, want nil for no timeout", job.Spec.ActiveDeadlineSeconds)
	}
}

func TestK8sRuntime_Create_APIError(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("create", "jobs", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("api server unavailable")
	})
	rt := NewK8sRuntimeFromClient(cs, "test-ns", "")

	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsRetryable(err) {
		t.Errorf("expected retryable error, got: %v", err)
	}
}

func TestK8sRuntime_Create_UniqueNames(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	ctx := context.Background()

	names := make(map[string]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for range 20 {
		wg.Go(func() {
			id, err := rt.Create(ctx, RunRequest{
				ImageURI:      "alpine:3.19",
				MachinePreset: "micro",
			})
			if err != nil {
				return
			}
			mu.Lock()
			names[id] = true
			mu.Unlock()
		})
	}
	wg.Wait()

	if len(names) < 15 { // Allow some collisions from fake clientset, but most should be unique.
		t.Errorf("only %d unique names from 20 creates, expected at least 15", len(names))
	}
}

func TestK8sRuntime_Create_NoPriorityClass(t *testing.T) {
	cs := fake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "test-ns", "")
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
	})

	job, _ := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	if job.Spec.Template.Spec.PriorityClassName != "" {
		t.Errorf("PriorityClassName = %q, want empty", job.Spec.Template.Spec.PriorityClassName)
	}
}

func TestK8sRuntime_Start_ReturnsErrMachineGone(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	err := rt.Start(context.Background(), "any-id", map[string]string{"FOO": "bar"})
	if !errors.Is(err, ErrMachineGone) {
		t.Errorf("Start() error = %v, want ErrMachineGone", err)
	}
}

func TestK8sRuntime_Destroy_DeletesJob(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	// Create a job first.
	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
	})

	err := rt.Destroy(ctx, id)
	if err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}

	// Verify job is deleted.
	_, err = cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	if err == nil {
		t.Error("expected job to be deleted")
	}
}

func TestK8sRuntime_Destroy_NotFound(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	err := rt.Destroy(context.Background(), "strait-aaaaaaaaaaaa")
	if !errors.Is(err, ErrMachineGone) {
		t.Errorf("Destroy(nonexistent) error = %v, want ErrMachineGone", err)
	}
}

func TestK8sRuntime_Destroy_Idempotent(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
	})

	_ = rt.Destroy(ctx, id)
	err := rt.Destroy(ctx, id)
	if !errors.Is(err, ErrMachineGone) {
		t.Errorf("second Destroy() error = %v, want ErrMachineGone", err)
	}
}

func TestK8sRuntime_Stop_DeletesJob(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
	})

	err := rt.Stop(ctx, id)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	_, err = cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	if err == nil {
		t.Error("expected job to be deleted after Stop")
	}
}

func TestK8sRuntime_Stop_NotFound(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	err := rt.Stop(context.Background(), "strait-aaaaaaaaaaaa")
	if !errors.Is(err, ErrMachineGone) {
		t.Errorf("Stop(nonexistent) error = %v, want ErrMachineGone", err)
	}
}

func TestK8sRuntime_Status_Pending(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro"})
	createPodForJob(t, cs, "test-ns", id, corev1.PodPending)

	status, err := rt.Status(ctx, id)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status != MachineStatusCreated {
		t.Errorf("Status() = %v, want Created", status)
	}
}

func TestK8sRuntime_Status_Running(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro"})
	createPodForJob(t, cs, "test-ns", id, corev1.PodRunning)

	status, _ := rt.Status(ctx, id)
	if status != MachineStatusRunning {
		t.Errorf("Status() = %v, want Running", status)
	}
}

func TestK8sRuntime_Status_Succeeded(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro"})
	createPodForJob(t, cs, "test-ns", id, corev1.PodSucceeded)

	status, _ := rt.Status(ctx, id)
	if status != MachineStatusStopped {
		t.Errorf("Status() = %v, want Stopped", status)
	}
}

func TestK8sRuntime_Status_Failed(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro"})
	createPodForJob(t, cs, "test-ns", id, corev1.PodFailed)

	status, _ := rt.Status(ctx, id)
	if status != MachineStatusStopped {
		t.Errorf("Status() = %v, want Stopped", status)
	}
}

func TestK8sRuntime_Status_NoPod(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	status, _ := rt.Status(context.Background(), "strait-aaaaaaaaaaaa")
	if status != MachineStatusUnknown {
		t.Errorf("Status() = %v, want Unknown", status)
	}
}

func TestK8sRuntime_Status_InvalidID(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	_, err := rt.Status(context.Background(), "malicious,app!=x")
	if err == nil {
		t.Error("expected error for invalid machineID")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got: %v", err)
	}
}

// K8sResources tests.

func TestK8sResources_Micro(t *testing.T) {
	p := AllPresets["micro"]
	req, lim := p.K8sResources()

	cpuReq := req[corev1.ResourceCPU]
	cpuLim := lim[corev1.ResourceCPU]
	memReq := req[corev1.ResourceMemory]

	if cpuReq.Cmp(resource.MustParse("100m")) != 0 {
		t.Errorf("micro CPU request = %s, want 100m", cpuReq.String())
	}
	if cpuLim.Cmp(resource.MustParse("1000m")) != 0 {
		t.Errorf("micro CPU limit = %s, want 1000m", cpuLim.String())
	}
	if memReq.Cmp(resource.MustParse("256Mi")) != 0 {
		t.Errorf("micro memory = %s, want 256Mi", memReq.String())
	}
}

func TestK8sResources_Medium1x(t *testing.T) {
	p := AllPresets["medium-1x"]
	req, lim := p.K8sResources()

	cpuReq := req[corev1.ResourceCPU]
	cpuLim := lim[corev1.ResourceCPU]

	// Guaranteed: request == limit.
	if cpuReq.Cmp(cpuLim) != 0 {
		t.Errorf("medium-1x CPU request (%s) != limit (%s)", cpuReq.String(), cpuLim.String())
	}
	if cpuReq.Cmp(resource.MustParse("2000m")) != 0 {
		t.Errorf("medium-1x CPU = %s, want 2000m", cpuReq.String())
	}
}

// Test helpers.

func createPodForJob(t *testing.T, cs *fake.Clientset, ns, jobName string, phase corev1.PodPhase) {
	t.Helper()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName + "-pod",
			Namespace: ns,
			Labels:    map[string]string{"job-name": jobName},
		},
		Status: corev1.PodStatus{
			Phase: phase,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "job",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 0,
						},
					},
				},
			},
		},
	}
	_, err := cs.CoreV1().Pods(ns).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create test pod: %v", err)
	}
}

// Security tests.

func TestValidateMachineID(t *testing.T) {
	valid := []string{
		"strait-abcdef012345",
		"strait-000000000000",
		"strait-abcdef123456",
	}
	for _, id := range valid {
		if err := validateMachineID(id); err != nil {
			t.Errorf("validateMachineID(%q) = %v, want nil", id, err)
		}
	}

	invalid := []string{
		"",
		"bad-id",
		"strait-short",                          // too short
		"strait-abcdef0123456",                  // too long (13 chars)
		"strait-ABCDEF012345",                   // uppercase
		"strait-abcdef01234g",                   // non-hex
		"other-abcdef012345",                    // wrong prefix
		"strait-abc,app!=x",                     // injection attempt
		"strait-abc\njob-name=x",                // newline injection
		"strait-abcdef012345,job-name=other-id", // comma injection
	}
	for _, id := range invalid {
		if err := validateMachineID(id); err == nil {
			t.Errorf("validateMachineID(%q) = nil, want error", id)
		}
	}
}

func TestSanitizeUserLabels(t *testing.T) {
	input := map[string]string{
		"project":  "my-project",
		"app":      "malicious-override",
		"job-name": "fake-job",
		"custom":   "ok",
	}
	got := sanitizeUserLabels(input)

	if _, ok := got["app"]; ok {
		t.Error("sanitizeUserLabels did not remove reserved key 'app'")
	}
	if _, ok := got["job-name"]; ok {
		t.Error("sanitizeUserLabels did not remove reserved key 'job-name'")
	}
	if got["project"] != "my-project" {
		t.Errorf("sanitizeUserLabels removed non-reserved key 'project'")
	}
	if got["custom"] != "ok" {
		t.Errorf("sanitizeUserLabels removed non-reserved key 'custom'")
	}
}

func TestSanitizeUserLabels_Nil(t *testing.T) {
	got := sanitizeUserLabels(nil)
	if got != nil {
		t.Errorf("sanitizeUserLabels(nil) = %v, want nil", got)
	}
}

func TestK8sRuntime_Create_SecurityContext(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	job, _ := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	c := job.Spec.Template.Spec.Containers[0]

	if c.SecurityContext == nil {
		t.Fatal("SecurityContext is nil")
	}
	if c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Error("AllowPrivilegeEscalation should be false")
	}
	if c.SecurityContext.Capabilities == nil || len(c.SecurityContext.Capabilities.Drop) == 0 {
		t.Error("Capabilities.Drop should contain ALL")
	}
}

func TestK8sRuntime_Create_ServiceAccount(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
	})

	job, _ := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	spec := job.Spec.Template.Spec

	if spec.ServiceAccountName != "strait-job-runner" {
		t.Errorf("ServiceAccountName = %q, want strait-job-runner", spec.ServiceAccountName)
	}
	if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		t.Error("AutomountServiceAccountToken should be false")
	}
}

func TestK8sRuntime_Create_ReservedLabelsStripped(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		Labels:        map[string]string{"app": "evil", "job-name": "evil", "custom": "ok"},
	})

	job, _ := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})

	if job.Labels["app"] != "strait-job" {
		t.Errorf("job label app = %q, want strait-job (internal override)", job.Labels["app"])
	}
	if job.Labels["custom"] != "ok" {
		t.Errorf("job label custom = %q, want ok", job.Labels["custom"])
	}
}

func TestK8sRuntime_Create_JobNameLength(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	ctx := context.Background()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Should be strait- + 12 hex chars = 19 chars total.
	if len(id) != 19 {
		t.Errorf("machineID length = %d, want 19 (strait- + 12 hex)", len(id))
	}
	if err := validateMachineID(id); err != nil {
		t.Errorf("Create() returned invalid machineID: %v", err)
	}
}

// Suppress unused import warnings.
var _ = (*batchv1.Job)(nil)
