package compute

import (
	"context"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func newSecurityTestRuntime() (*K8sRuntime, *k8sfake.Clientset) {
	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "strait-job")
	return rt, cs
}

func createJobAndGetSpec(t *testing.T, rt *K8sRuntime, cs *k8sfake.Clientset) *batchv1.Job {
	t.Helper()
	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "micro",
		Env:           map[string]string{"TEST": "1"},
		Labels:        map[string]string{"run_id": "test-123"},
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	jobs, err := cs.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(jobs.Items) == 0 {
		t.Fatal("no jobs created")
	}
	return &jobs.Items[0]
}

func TestK8s_JobPodSecurityContext(t *testing.T) {
	t.Parallel()
	rt, cs := newSecurityTestRuntime()
	job := createJobAndGetSpec(t, rt, cs)
	podSpec := job.Spec.Template.Spec

	// Pod-level security context.
	psc := podSpec.SecurityContext
	if psc == nil {
		t.Fatal("pod security context is nil")
	}
	if psc.RunAsNonRoot == nil || !*psc.RunAsNonRoot {
		t.Error("RunAsNonRoot must be true")
	}
	if psc.RunAsUser == nil || *psc.RunAsUser != 65534 {
		t.Errorf("RunAsUser = %v, want 65534 (nobody)", psc.RunAsUser)
	}
	if psc.SeccompProfile == nil || psc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("SeccompProfile must be RuntimeDefault")
	}

	// Container-level security context.
	csc := podSpec.Containers[0].SecurityContext
	if csc == nil {
		t.Fatal("container security context is nil")
	}
	if csc.AllowPrivilegeEscalation == nil || *csc.AllowPrivilegeEscalation {
		t.Error("AllowPrivilegeEscalation must be false")
	}
	if csc.ReadOnlyRootFilesystem == nil || !*csc.ReadOnlyRootFilesystem {
		t.Error("ReadOnlyRootFilesystem must be true")
	}
	if csc.Capabilities == nil || len(csc.Capabilities.Drop) == 0 {
		t.Error("must drop ALL capabilities")
	} else if csc.Capabilities.Drop[0] != "ALL" {
		t.Errorf("Capabilities.Drop = %v, want [ALL]", csc.Capabilities.Drop)
	}
}

func TestK8s_JobPodNoServiceAccountToken(t *testing.T) {
	t.Parallel()
	rt, cs := newSecurityTestRuntime()
	job := createJobAndGetSpec(t, rt, cs)
	podSpec := job.Spec.Template.Spec

	if podSpec.AutomountServiceAccountToken == nil || *podSpec.AutomountServiceAccountToken {
		t.Error("AutomountServiceAccountToken must be false (prevents K8s API access)")
	}
	if podSpec.ServiceAccountName != "strait-job-runner" {
		t.Errorf("ServiceAccountName = %q, want strait-job-runner", podSpec.ServiceAccountName)
	}
}

func TestK8s_JobPodLabels(t *testing.T) {
	t.Parallel()
	rt, cs := newSecurityTestRuntime()
	job := createJobAndGetSpec(t, rt, cs)

	if job.Labels["app"] != "strait-job" {
		t.Errorf("job label app = %q, want strait-job", job.Labels["app"])
	}
	if job.Spec.Template.Labels["app"] != "strait-job" {
		t.Errorf("pod template label app = %q, want strait-job", job.Spec.Template.Labels["app"])
	}
}

func TestK8s_JobPodTTL(t *testing.T) {
	t.Parallel()
	rt, cs := newSecurityTestRuntime()
	job := createJobAndGetSpec(t, rt, cs)

	if job.Spec.TTLSecondsAfterFinished == nil {
		t.Fatal("TTLSecondsAfterFinished must be set")
	}
	if *job.Spec.TTLSecondsAfterFinished != int32(jobTTLAfterFinished) {
		t.Errorf("TTLSecondsAfterFinished = %d, want %d", *job.Spec.TTLSecondsAfterFinished, jobTTLAfterFinished)
	}
}

func TestK8s_ImageURIValidation_Adversarial(t *testing.T) {
	t.Parallel()
	malicious := []string{
		"",
		" ",
		"../../../etc/passwd",
		"alpine; rm -rf /",
		"alpine\nRUN echo hacked",
		"alpine$(whoami)",
		"alpine`id`",
		"alpine\x00injected",
		"https://evil.com/malware",
		"file:///etc/passwd",
		strings.Repeat("a", 1000),
		"alpine:tag:extra:colons",
		"ALLCAPS:LATEST",
	}

	for _, uri := range malicious {
		err := validateImageURI(uri)
		if err == nil {
			t.Errorf("validateImageURI(%q) should have failed", uri)
		}
	}
}

func TestK8s_LabelInjection_Adversarial(t *testing.T) {
	t.Parallel()
	adversarial := map[string]string{
		"app":                      "malicious-override", // should not override system label
		"'; DROP TABLE jobs; --":   "sqli",
		"../../etc/passwd":         "path-traversal",
		"key\x00null":              "null-byte",
		strings.Repeat("x", 100):   "long-key",
		"valid-key":                strings.Repeat("y", 100),
		"kubernetes.io/managed-by": "k8s-reserved",
	}

	sanitized := sanitizeUserLabels(adversarial)

	// System label must not be overridable.
	rt := NewK8sRuntimeFromClient(k8sfake.NewSimpleClientset(), "default", "")
	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "micro",
		Labels:        adversarial,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	jobs, _ := rt.clientset.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) == 0 {
		t.Fatal("no jobs created")
	}

	// The system "app" label must always be "strait-job", never user-overridden.
	if jobs.Items[0].Labels["app"] != "strait-job" {
		t.Errorf("system label 'app' was overridden to %q", jobs.Items[0].Labels["app"])
	}

	// Sanitized labels should not contain dangerous characters.
	for k, v := range sanitized {
		if strings.ContainsAny(k, "\x00\n\r") {
			t.Errorf("sanitized key contains dangerous chars: %q", k)
		}
		if strings.ContainsAny(v, "\x00\n\r") {
			t.Errorf("sanitized value contains dangerous chars: %q", v)
		}
	}
}

func TestK8s_NamespaceIsolation(t *testing.T) {
	t.Parallel()

	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "production-jobs", "")

	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "micro",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Job must be in the configured namespace, not "default".
	jobs, _ := cs.BatchV1().Jobs("production-jobs").List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 1 {
		t.Fatalf("expected 1 job in production-jobs namespace, got %d", len(jobs.Items))
	}

	// No jobs in default namespace.
	defaultJobs, _ := cs.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{})
	if len(defaultJobs.Items) != 0 {
		t.Fatalf("expected 0 jobs in default namespace, got %d", len(defaultJobs.Items))
	}
}

func TestK8s_ResourceLimits_AllPresets(t *testing.T) {
	t.Parallel()

	for name, preset := range AllPresets {
		requests, limits := preset.K8sResources()
		if requests.Cpu().IsZero() {
			t.Errorf("preset %q: CPU request is zero", name)
		}
		if requests.Memory().IsZero() {
			t.Errorf("preset %q: memory request is zero", name)
		}
		if limits.Cpu().IsZero() {
			t.Errorf("preset %q: CPU limit is zero", name)
		}
		if limits.Memory().IsZero() {
			t.Errorf("preset %q: memory limit is zero", name)
		}
		// Requests must not exceed limits.
		if requests.Cpu().Cmp(*limits.Cpu()) > 0 {
			t.Errorf("preset %q: CPU request (%v) exceeds limit (%v)", name, requests.Cpu(), limits.Cpu())
		}
		if requests.Memory().Cmp(*limits.Memory()) > 0 {
			t.Errorf("preset %q: memory request (%v) exceeds limit (%v)", name, requests.Memory(), limits.Memory())
		}
	}
}

// Ensure job pods cannot mount host paths or use privileged containers.
func TestK8s_JobPodNoHostAccess(t *testing.T) {
	t.Parallel()
	rt, cs := newSecurityTestRuntime()
	job := createJobAndGetSpec(t, rt, cs)
	podSpec := job.Spec.Template.Spec

	// No host network, PID, or IPC.
	if podSpec.HostNetwork {
		t.Error("HostNetwork must be false")
	}
	if podSpec.HostPID {
		t.Error("HostPID must be false")
	}
	if podSpec.HostIPC {
		t.Error("HostIPC must be false")
	}

	// No volumes mounted (job pods should be stateless).
	if len(podSpec.Volumes) != 0 {
		t.Errorf("expected 0 volumes, got %d", len(podSpec.Volumes))
	}
}

// Verify the fake clientset captures all operations correctly.
func TestK8s_SecurityContext_ConsistentAcrossPresets(t *testing.T) {
	t.Parallel()

	presets := []string{"micro", "small-1x", "medium-1x", "large-1x"}
	for _, preset := range presets {
		cs := k8sfake.NewSimpleClientset()
		rt := NewK8sRuntimeFromClient(cs, "default", "strait-job")

		_, err := rt.Create(context.Background(), RunRequest{
			ImageURI:      "alpine:3.21",
			MachinePreset: preset,
		})
		if err != nil {
			t.Fatalf("preset %s: Create failed: %v", preset, err)
		}

		jobs, _ := cs.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{})
		if len(jobs.Items) == 0 {
			t.Fatalf("preset %s: no jobs", preset)
		}

		psc := jobs.Items[0].Spec.Template.Spec.SecurityContext
		if psc == nil || psc.RunAsNonRoot == nil || !*psc.RunAsNonRoot {
			t.Errorf("preset %s: RunAsNonRoot not enforced", preset)
		}
	}
}

// Suppress unused import warnings.
var _ = runtime.Object(nil)
