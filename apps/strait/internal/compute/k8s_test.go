package compute

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
	rt := NewK8sRuntimeFromClient(cs, "test-ns", "strait-job", "")
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

func TestK8sRuntime_Create_NoTimeout_UsesDefault(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		TimeoutSecs:   0,
	})

	job, _ := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	if job.Spec.ActiveDeadlineSeconds == nil {
		t.Fatal("ActiveDeadlineSeconds is nil, want default max deadline")
	}
	if *job.Spec.ActiveDeadlineSeconds != defaultMaxDeadlineSecs {
		t.Errorf("ActiveDeadlineSeconds = %d, want %d (default max)", *job.Spec.ActiveDeadlineSeconds, defaultMaxDeadlineSecs)
	}
}

func TestK8sRuntime_Create_APIError(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("create", "jobs", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("api server unavailable")
	})
	rt := NewK8sRuntimeFromClient(cs, "test-ns", "", "")

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
	rt := NewK8sRuntimeFromClient(cs, "test-ns", "", "")
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
	podSpec := job.Spec.Template.Spec
	c := podSpec.Containers[0]

	// Container-level security.
	if c.SecurityContext == nil {
		t.Fatal("container SecurityContext is nil")
	}
	if c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Error("AllowPrivilegeEscalation should be false")
	}
	if c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("ReadOnlyRootFilesystem should be true")
	}
	if c.SecurityContext.Capabilities == nil || len(c.SecurityContext.Capabilities.Drop) == 0 {
		t.Error("Capabilities.Drop should contain ALL")
	}

	// Pod-level security.
	if podSpec.SecurityContext == nil {
		t.Fatal("pod SecurityContext is nil")
	}
	if podSpec.SecurityContext.RunAsNonRoot == nil || !*podSpec.SecurityContext.RunAsNonRoot {
		t.Error("RunAsNonRoot should be true")
	}
	if podSpec.SecurityContext.RunAsUser == nil || *podSpec.SecurityContext.RunAsUser != 65534 {
		t.Errorf("RunAsUser = %v, want 65534 (nobody)", podSpec.SecurityContext.RunAsUser)
	}
	if podSpec.SecurityContext.SeccompProfile == nil || podSpec.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("SeccompProfile should be RuntimeDefault")
	}

	// TTL safety net.
	if job.Spec.TTLSecondsAfterFinished == nil {
		t.Error("TTLSecondsAfterFinished should be set")
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

func TestK8sRuntime_Create_DefaultNamespace(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		// Namespace empty -- should use default from runtime ("test-ns").
	})

	job, _ := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	if job == nil {
		t.Fatal("job not found in default namespace")
	}
}

func TestK8sRuntime_Create_OverrideNamespace(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
		Namespace:     "tenant-abc",
	})

	// Job should be in the override namespace.
	job, err := cs.BatchV1().Jobs("tenant-abc").Get(ctx, id, metav1.GetOptions{})
	if err != nil || job == nil {
		t.Fatalf("job not found in override namespace tenant-abc: %v", err)
	}

	// Job should NOT be in the default namespace.
	_, err = cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	if err == nil {
		t.Error("job found in default namespace, should be in tenant-abc")
	}
}

func TestK8sRuntime_Create_NodeAffinity_AllPresets(t *testing.T) {
	tests := []struct {
		preset   string
		wantPool string
	}{
		{"micro", NodePoolGeneral},
		{"small-1x", NodePoolGeneral},
		{"small-2x", NodePoolGeneral},
		{"medium-1x", NodePoolPerformance},
		{"medium-2x", NodePoolPerformance},
		{"large-1x", NodePoolHeavy},
		{"large-2x", NodePoolHeavy},
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			rt, cs := newTestK8sRuntime()
			ctx := context.Background()

			id, err := rt.Create(ctx, RunRequest{
				ImageURI:      "alpine:3.19",
				MachinePreset: tt.preset,
				TimeoutSecs:   30,
			})
			if err != nil {
				t.Fatalf("Create(%s): %v", tt.preset, err)
			}

			job, _ := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
			affinity := job.Spec.Template.Spec.Affinity
			if affinity == nil {
				t.Fatalf("preset %s: Affinity is nil", tt.preset)
			}
			if affinity.NodeAffinity == nil {
				t.Fatalf("preset %s: NodeAffinity is nil", tt.preset)
			}

			terms := affinity.NodeAffinity.PreferredDuringSchedulingIgnoredDuringExecution
			if len(terms) != 1 {
				t.Fatalf("preset %s: expected 1 term, got %d", tt.preset, len(terms))
			}

			pool := terms[0].Preference.MatchExpressions[0].Values[0]
			if pool != tt.wantPool {
				t.Errorf("preset %s: pool=%q, want %q", tt.preset, pool, tt.wantPool)
			}

			// Verify it's soft (preferred), not hard (required).
			if affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
				t.Errorf("preset %s: has hard affinity, want soft only", tt.preset)
			}
		})
	}
}

func TestK8sRuntime_Create_InvalidImageURI(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI:      "image;rm -rf /",
		MachinePreset: "micro",
	})
	if err == nil {
		t.Fatal("expected error for malicious image URI")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got: %v", err)
	}
}

func TestK8sRuntime_Create_TTLSecondsAfterFinished(t *testing.T) {
	rt, cs := newTestK8sRuntime()
	ctx := context.Background()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.19",
		MachinePreset: "micro",
	})

	job, _ := cs.BatchV1().Jobs("test-ns").Get(ctx, id, metav1.GetOptions{})
	if job.Spec.TTLSecondsAfterFinished == nil {
		t.Fatal("TTLSecondsAfterFinished is nil")
	}
	if *job.Spec.TTLSecondsAfterFinished != jobTTLAfterFinished {
		t.Errorf("TTLSecondsAfterFinished = %d, want %d", *job.Spec.TTLSecondsAfterFinished, jobTTLAfterFinished)
	}
}

func TestK8sRuntime_Create_SetsImagePullPolicyIfNotPresent(t *testing.T) {
	t.Parallel()
	cs := fake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "", string(corev1.PullIfNotPresent))
	ctx := context.Background()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "micro",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	job, err := cs.BatchV1().Jobs("default").Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get job error = %v", err)
	}
	got := job.Spec.Template.Spec.Containers[0].ImagePullPolicy
	if got != corev1.PullIfNotPresent {
		t.Errorf("ImagePullPolicy = %q, want %q", got, corev1.PullIfNotPresent)
	}
}

func TestK8sRuntime_Create_SetsImagePullPolicyAlways(t *testing.T) {
	t.Parallel()
	cs := fake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "", string(corev1.PullAlways))
	ctx := context.Background()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "micro",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	job, err := cs.BatchV1().Jobs("default").Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get job error = %v", err)
	}
	got := job.Spec.Template.Spec.Containers[0].ImagePullPolicy
	if got != corev1.PullAlways {
		t.Errorf("ImagePullPolicy = %q, want %q", got, corev1.PullAlways)
	}
}

func TestK8sRuntime_Create_EmptyPullPolicyLeavesDefault(t *testing.T) {
	t.Parallel()
	cs := fake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "", "")
	ctx := context.Background()

	id, err := rt.Create(ctx, RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "micro",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	job, err := cs.BatchV1().Jobs("default").Get(ctx, id, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Get job error = %v", err)
	}
	got := job.Spec.Template.Spec.Containers[0].ImagePullPolicy
	if got != corev1.PullPolicy("") {
		t.Errorf("ImagePullPolicy = %q, want empty (kubernetes default)", got)
	}
}

// Suppress unused import warnings.
var _ = (*batchv1.Job)(nil)

// mockK8sMetricsUnit is used for unit testing K8sRuntime metrics paths.
// Note: k8s_integration_test.go defines mockK8sMetrics; this uses a distinct name.

type mockK8sMetricsUnit struct {
	jobCreates      []string // "status:preset"
	jobWaits        []string // "exitStatus"
	podScheduling   []float64
	jobsActiveDelta int64
}

func (m *mockK8sMetricsUnit) RecordJobCreate(status, preset string, _ float64) {
	m.jobCreates = append(m.jobCreates, status+":"+preset)
}
func (m *mockK8sMetricsUnit) RecordJobWait(exitStatus string, _ float64) {
	m.jobWaits = append(m.jobWaits, exitStatus)
}
func (m *mockK8sMetricsUnit) RecordPodScheduling(secs float64) {
	m.podScheduling = append(m.podScheduling, secs)
}
func (m *mockK8sMetricsUnit) IncJobsActive(delta int64) {
	m.jobsActiveDelta += delta
}

// handlePodPhase coverage.

// TestHandlePodPhase_Succeeded_CompletesWithMetrics verifies that PodSucceeded
// returns podActionComplete and records metrics.
func TestHandlePodPhase_Succeeded_CompletesWithMetrics(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	m := &mockK8sMetricsUnit{}
	rt.SetMetrics(m)

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "job",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
					},
				},
			},
		},
	}
	ws := &waitState{result: &RunResult{MachineID: "strait-aabbcc001122"}}
	action := rt.handlePodPhase(context.Background(), "strait-aabbcc001122", pod, ws)

	if action != podActionComplete {
		t.Errorf("handlePodPhase(Succeeded) = %v, want podActionComplete", action)
	}
	if len(m.jobWaits) == 0 || m.jobWaits[0] != "success" {
		t.Errorf("expected RecordJobWait(success), got %v", m.jobWaits)
	}
}

// TestHandlePodPhase_Failed_NonZeroExit_RecordsFailureMetric verifies that
// PodFailed with a non-zero exit code records "failure" metric.
func TestHandlePodPhase_Failed_NonZeroExit_RecordsFailureMetric(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	m := &mockK8sMetricsUnit{}
	rt.SetMetrics(m)

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "job",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{ExitCode: 1},
					},
				},
			},
		},
	}
	ws := &waitState{result: &RunResult{MachineID: "strait-aabbcc001123"}}
	action := rt.handlePodPhase(context.Background(), "strait-aabbcc001123", pod, ws)

	if action != podActionComplete {
		t.Errorf("handlePodPhase(Failed) = %v, want podActionComplete", action)
	}
	if len(m.jobWaits) == 0 || m.jobWaits[0] != "failure" {
		t.Errorf("expected RecordJobWait(failure), got %v", m.jobWaits)
	}
}

// TestHandlePodPhase_Failed_OOMKilled_RecordsOOMMetric verifies that a pod
// killed by OOM records "oom" metric.
func TestHandlePodPhase_Failed_OOMKilled_RecordsOOMMetric(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	m := &mockK8sMetricsUnit{}
	rt.SetMetrics(m)

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "job",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 137,
							Reason:   "OOMKilled",
						},
					},
				},
			},
		},
	}
	ws := &waitState{result: &RunResult{MachineID: "strait-aabbcc001124"}}
	action := rt.handlePodPhase(context.Background(), "strait-aabbcc001124", pod, ws)

	if action != podActionComplete {
		t.Errorf("expected podActionComplete, got %v", action)
	}
	if !ws.result.OOMKilled {
		t.Error("expected OOMKilled=true")
	}
	if len(m.jobWaits) == 0 || m.jobWaits[0] != "oom" {
		t.Errorf("expected RecordJobWait(oom), got %v", m.jobWaits)
	}
}

// TestHandlePodPhase_Running_FirstTime_RecordsScheduling verifies that the
// first time a pod enters Running, podRunningAt is set and RecordPodScheduling
// is called.
func TestHandlePodPhase_Running_FirstTime_RecordsScheduling(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	m := &mockK8sMetricsUnit{}
	rt.SetMetrics(m)

	pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	ws := &waitState{
		result:    &RunResult{MachineID: "strait-aabbcc001125"},
		waitStart: time.Now().Add(-5 * time.Second),
		// podRunningAt is zero
	}

	action := rt.handlePodPhase(context.Background(), "strait-aabbcc001125", pod, ws)

	if action != podActionContinue {
		t.Errorf("expected podActionContinue, got %v", action)
	}
	if ws.podRunningAt.IsZero() {
		t.Error("expected podRunningAt to be set on first Running observation")
	}
	if len(m.podScheduling) == 0 {
		t.Error("expected RecordPodScheduling to be called on first Running")
	}
}

// TestHandlePodPhase_Running_Subsequent_NoNewSchedulingRecord verifies that
// subsequent Running observations do not call RecordPodScheduling again.
func TestHandlePodPhase_Running_Subsequent_NoNewSchedulingRecord(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	m := &mockK8sMetricsUnit{}
	rt.SetMetrics(m)

	pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}
	alreadyRunning := time.Now().Add(-10 * time.Second)
	ws := &waitState{
		result:       &RunResult{MachineID: "strait-aabbcc001126"},
		waitStart:    alreadyRunning.Add(-5 * time.Second),
		podRunningAt: alreadyRunning, // already set
	}

	rt.handlePodPhase(context.Background(), "strait-aabbcc001126", pod, ws)
	rt.handlePodPhase(context.Background(), "strait-aabbcc001126", pod, ws)

	if len(m.podScheduling) != 0 {
		t.Errorf("expected RecordPodScheduling NOT called on subsequent Running, got %d calls", len(m.podScheduling))
	}
}

// TestHandlePodPhase_Pending_WithFailureReason_ReturnsFatal verifies that a
// pending pod with an unschedulable condition returns podActionFatal.
func TestHandlePodPhase_Pending_WithFailureReason_ReturnsFatal(t *testing.T) {
	rt, _ := newTestK8sRuntime()

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{
				{
					Type:    corev1.PodScheduled,
					Status:  corev1.ConditionFalse,
					Reason:  "Unschedulable",
					Message: "insufficient memory",
				},
			},
		},
	}
	ws := &waitState{result: &RunResult{MachineID: "strait-aabbcc001127"}}
	action := rt.handlePodPhase(context.Background(), "strait-aabbcc001127", pod, ws)

	if action != podActionFatal {
		t.Errorf("expected podActionFatal for unschedulable pod, got %v", action)
	}
}

// TestHandlePodPhase_Pending_NoReason_ReturnsContinue verifies that a pending
// pod without a failure reason returns podActionContinue.
func TestHandlePodPhase_Pending_NoReason_ReturnsContinue(t *testing.T) {
	rt, _ := newTestK8sRuntime()

	pod := &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}}
	ws := &waitState{result: &RunResult{MachineID: "strait-aabbcc001128"}}
	action := rt.handlePodPhase(context.Background(), "strait-aabbcc001128", pod, ws)

	if action != podActionContinue {
		t.Errorf("expected podActionContinue for pending pod without failure, got %v", action)
	}
}

// TestHandlePodPhase_NilMetrics_NoPanic verifies that handlePodPhase does not
// panic when metrics is nil (default nil-safe path).
func TestHandlePodPhase_NilMetrics_NoPanic(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	// metrics is nil by default

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "job", State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{ExitCode: 0},
				}},
			},
		},
	}
	ws := &waitState{result: &RunResult{MachineID: "strait-aabbcc001129"}}
	// Must not panic.
	_ = rt.handlePodPhase(context.Background(), "strait-aabbcc001129", pod, ws)
}

// extractExitInfo coverage.

// TestExtractExitInfo_NoJobContainer_LeavesDefaults verifies that when no
// container named "job" exists, RunResult fields stay at zero values.
func TestExtractExitInfo_NoJobContainer_LeavesDefaults(t *testing.T) {
	rt, _ := newTestK8sRuntime()

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "sidecar", State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{ExitCode: 1},
				}},
			},
		},
	}
	result := &RunResult{}
	rt.extractExitInfo(pod, result)

	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0 (no job container)", result.ExitCode)
	}
}

// TestExtractExitInfo_SignalTermination_SetsExitSignal verifies that a container
// killed by signal > 0 sets ExitSignal.
func TestExtractExitInfo_SignalTermination_SetsExitSignal(t *testing.T) {
	rt, _ := newTestK8sRuntime()

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "job", State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 137,
						Signal:   9,
					},
				}},
			},
		},
	}
	result := &RunResult{}
	rt.extractExitInfo(pod, result)

	if result.ExitSignal == "" {
		t.Error("expected ExitSignal to be set for signal termination")
	}
	if result.ExitCode != 137 {
		t.Errorf("ExitCode = %d, want 137", result.ExitCode)
	}
}

// TestExtractExitInfo_OOMKilled_SetsFlag verifies that OOMKilled reason sets
// result.OOMKilled = true.
func TestExtractExitInfo_OOMKilled_SetsFlag(t *testing.T) {
	rt, _ := newTestK8sRuntime()

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "job", State: corev1.ContainerState{
					Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 137,
						Reason:   "OOMKilled",
					},
				}},
			},
		},
	}
	result := &RunResult{}
	rt.extractExitInfo(pod, result)

	if !result.OOMKilled {
		t.Error("expected OOMKilled=true for OOMKilled reason")
	}
}

// TestExtractExitInfo_TerminatedNil_LeavesDefaults verifies that when
// cs.State.Terminated is nil the result fields stay at zero values.
func TestExtractExitInfo_TerminatedNil_LeavesDefaults(t *testing.T) {
	rt, _ := newTestK8sRuntime()

	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "job", State: corev1.ContainerState{
					// Terminated is nil — container is still running or waiting.
				}},
			},
		},
	}
	result := &RunResult{}
	rt.extractExitInfo(pod, result)

	if result.ExitCode != 0 || result.OOMKilled || result.ExitSignal != "" {
		t.Errorf("expected zero values, got ExitCode=%d OOMKilled=%v ExitSignal=%q",
			result.ExitCode, result.OOMKilled, result.ExitSignal)
	}
}

// recordCreateError and recordWaitTimeout nil-metrics coverage.

// TestRecordCreateError_NilMetrics_NoPanic verifies recordCreateError is nil-safe.
func TestRecordCreateError_NilMetrics_NoPanic(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	// metrics is nil by default
	// Must not panic.
	rt.recordCreateError("small-1x", time.Now().Add(-time.Second))
}

// TestRecordCreateError_WithMetrics_RecordsCounter verifies that recordCreateError
// calls RecordJobCreate with status="error".
func TestRecordCreateError_WithMetrics_RecordsCounter(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	m := &mockK8sMetricsUnit{}
	rt.SetMetrics(m)

	rt.recordCreateError("small-1x", time.Now().Add(-time.Second))

	if len(m.jobCreates) == 0 {
		t.Fatal("expected RecordJobCreate to be called")
	}
	if m.jobCreates[0] != "error:small-1x" {
		t.Errorf("RecordJobCreate called with %q, want error:small-1x", m.jobCreates[0])
	}
}

// TestRecordWaitTimeout_NilMetrics_NoPanic verifies recordWaitTimeout is nil-safe.
func TestRecordWaitTimeout_NilMetrics_NoPanic(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	// metrics is nil by default
	ws := &waitState{result: &RunResult{}, waitStart: time.Now().Add(-time.Second)}
	// Must not panic.
	rt.recordWaitTimeout(ws)
}

// TestRecordWaitTimeout_WithMetrics_RecordsTimeout verifies that recordWaitTimeout
// calls RecordJobWait with exitStatus="timeout".
func TestRecordWaitTimeout_WithMetrics_RecordsTimeout(t *testing.T) {
	rt, _ := newTestK8sRuntime()
	m := &mockK8sMetricsUnit{}
	rt.SetMetrics(m)

	ws := &waitState{result: &RunResult{}, waitStart: time.Now().Add(-5 * time.Second)}
	rt.recordWaitTimeout(ws)

	if len(m.jobWaits) == 0 || m.jobWaits[0] != "timeout" {
		t.Errorf("expected RecordJobWait(timeout), got %v", m.jobWaits)
	}
}
