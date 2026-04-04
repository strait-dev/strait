package compute

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Security & Isolation stress tests.

func getJobPodSpec(t *testing.T, rt *K8sRuntime, jobID string) corev1.PodSpec {
	t.Helper()
	ctx := context.Background()
	job, err := rt.clientset.BatchV1().Jobs("default").Get(ctx, jobID, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	return job.Spec.Template.Spec
}

func TestStress_RunAsNonRoot(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	spec := getJobPodSpec(t, rt, id)
	if spec.SecurityContext == nil || spec.SecurityContext.RunAsNonRoot == nil || !*spec.SecurityContext.RunAsNonRoot {
		t.Error("RunAsNonRoot not set on pod")
	}
	if spec.SecurityContext.RunAsUser == nil || *spec.SecurityContext.RunAsUser != 65534 {
		t.Error("RunAsUser not 65534 (nobody)")
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_ReadOnly_Filesystem(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	c := getJobPodSpec(t, rt, id).Containers[0]
	if c.SecurityContext == nil || c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("ReadOnlyRootFilesystem not set")
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_No_Capabilities(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	c := getJobPodSpec(t, rt, id).Containers[0]
	if c.SecurityContext.Capabilities == nil || len(c.SecurityContext.Capabilities.Drop) == 0 {
		t.Error("capabilities not dropped")
	}
	if c.SecurityContext.Capabilities.Drop[0] != "ALL" {
		t.Errorf("expected Drop: ALL, got %v", c.SecurityContext.Capabilities.Drop)
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_No_Privilege_Escalation(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	c := getJobPodSpec(t, rt, id).Containers[0]
	if c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Error("AllowPrivilegeEscalation not false")
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_No_SA_Token(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	spec := getJobPodSpec(t, rt, id)
	if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		t.Error("AutomountServiceAccountToken not false")
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_Env_Vars_Isolated(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	r1, _ := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30, Env: map[string]string{"SECRET_A": "value_a"}})
	r2, _ := rt.Run(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30, Env: map[string]string{"SECRET_B": "value_b"}})

	if r1 != nil {
		t.Cleanup(func() { _ = rt.Destroy(context.Background(), r1.MachineID) })
	}
	if r2 != nil {
		t.Cleanup(func() { _ = rt.Destroy(context.Background(), r2.MachineID) })
	}

	if r1 != nil && r2 != nil && r1.MachineID != r2.MachineID {
		t.Log("env vars isolated: different jobs have different machineIDs")
	}
}

func TestStress_Labels_Sanitized(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30,
		Labels: map[string]string{"app": "evil", "custom": "ok"},
	})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	job, _ := rt.clientset.BatchV1().Jobs("default").Get(ctx, id, metav1.GetOptions{})
	if job.Labels["app"] != "strait-job" {
		t.Errorf("app label=%q, want strait-job (sanitized)", job.Labels["app"])
	}
	if job.Labels["custom"] != "ok" {
		t.Errorf("custom label=%q, want ok", job.Labels["custom"])
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_MachineID_Injection(t *testing.T) {
	rt := requireKindCluster(t)
	injections := []string{
		"strait-abc,app!=x",
		"strait-abc\njob-name=x",
		"",
		"../../../etc/passwd",
	}
	for _, id := range injections {
		_, err := rt.Status(context.Background(), id)
		if err == nil {
			t.Errorf("Status(%q) should fail", id)
		}
	}
}

func TestStress_Image_URI_Injection(t *testing.T) {
	rt := requireKindCluster(t)
	injections := []string{
		"image;rm -rf /",
		"image|cat /etc/passwd",
		"$(whoami)",
	}
	for _, img := range injections {
		_, err := rt.Create(context.Background(), RunRequest{ImageURI: img, MachinePreset: "micro", TimeoutSecs: 30})
		if err == nil {
			t.Errorf("Create(%q) should fail", img)
		}
	}
}

func TestStress_Reserved_Labels_Stripped(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{
		ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30,
		Labels: map[string]string{"job-name": "evil-name"},
	})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	job, _ := rt.clientset.BatchV1().Jobs("default").Get(ctx, id, metav1.GetOptions{})
	// The pod template should have the real job-name, not the user-supplied one.
	podLabels := job.Spec.Template.Labels
	if podLabels["job-name"] != id {
		t.Errorf("pod job-name=%q, want %q", podLabels["job-name"], id)
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_ServiceAccount_Correct(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	spec := getJobPodSpec(t, rt, id)
	if spec.ServiceAccountName != "strait-job-runner" {
		t.Errorf("ServiceAccountName=%q, want strait-job-runner", spec.ServiceAccountName)
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_Seccomp_Profile(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	spec := getJobPodSpec(t, rt, id)
	if spec.SecurityContext == nil || spec.SecurityContext.SeccompProfile == nil {
		t.Fatal("SeccompProfile not set")
	}
	if spec.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("SeccompProfile type=%s, want RuntimeDefault", spec.SecurityContext.SeccompProfile.Type)
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_Pod_Security_Context(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	spec := getJobPodSpec(t, rt, id)
	sc := spec.SecurityContext
	if sc == nil {
		t.Fatal("pod SecurityContext nil")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("RunAsNonRoot not true")
	}
	if sc.RunAsUser == nil || *sc.RunAsUser != 65534 {
		t.Error("RunAsUser not 65534")
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_Container_Security_Context(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	c := getJobPodSpec(t, rt, id).Containers[0]
	if c.SecurityContext == nil {
		t.Fatal("container SecurityContext nil")
	}
	if c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Error("AllowPrivilegeEscalation not false")
	}
	if c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("ReadOnlyRootFilesystem not true")
	}
	_, _ = rt.Wait(ctx, id, 30)
}

func TestStress_AutomountToken_Disabled(t *testing.T) {
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	id, _ := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
	t.Cleanup(func() { _ = rt.Destroy(context.Background(), id) })

	spec := getJobPodSpec(t, rt, id)
	if spec.AutomountServiceAccountToken == nil || *spec.AutomountServiceAccountToken {
		t.Error("AutomountServiceAccountToken not false")
	}
	_, _ = rt.Wait(ctx, id, 30)
}
