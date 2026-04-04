package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// getClient returns a K8s clientset connected to the infra cluster.
func getClient(t *testing.T) *kubernetes.Clientset {
	t.Helper()

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		// Try the infra kubeconfig.
		kubeconfig = filepath.Join("..", "kubeconfig")
	}

	if _, err := os.Stat(kubeconfig); os.IsNotExist(err) {
		t.Skipf("kubeconfig not found at %s. Run: make infra-kubeconfig", kubeconfig)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("build kubeconfig: %v", err)
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}
	return cs
}

// TestClusterNodes verifies all nodes exist and are Ready.
func TestClusterNodes(t *testing.T) {
	cs := getClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}

	if len(nodes.Items) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(nodes.Items))
	}

	for _, node := range nodes.Items {
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
			}
		}
		if !ready {
			t.Errorf("node %s is not Ready", node.Name)
		}
		t.Logf("node %s: Ready, labels=%v", node.Name, node.Labels)
	}
}

// TestNodeLabels verifies workers have the strait.dev/pool label.
func TestNodeLabels(t *testing.T) {
	cs := getClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}

	poolCounts := map[string]int{}
	for _, node := range nodes.Items {
		pool, ok := node.Labels["strait.dev/pool"]
		if ok {
			poolCounts[pool]++
		}
	}

	t.Logf("node pool distribution: %v", poolCounts)

	if poolCounts["general"] < 1 {
		t.Error("expected at least 1 node with strait.dev/pool=general")
	}
}

// TestPriorityClasses verifies strait-job and warm-pool priority classes exist.
func TestPriorityClasses(t *testing.T) {
	cs := getClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pc, err := cs.SchedulingV1().PriorityClasses().Get(ctx, "strait-job", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get strait-job priority class: %v", err)
	}
	if pc.Value != 100 {
		t.Errorf("strait-job priority = %d, want 100", pc.Value)
	}

	wp, err := cs.SchedulingV1().PriorityClasses().Get(ctx, "warm-pool", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get warm-pool priority class: %v", err)
	}
	if wp.Value != -1 {
		t.Errorf("warm-pool priority = %d, want -1", wp.Value)
	}
}

// TestServiceAccount verifies strait-job-runner SA exists.
func TestServiceAccount(t *testing.T) {
	cs := getClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sa, err := cs.CoreV1().ServiceAccounts("default").Get(ctx, "strait-job-runner", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get strait-job-runner SA: %v", err)
	}
	if sa.AutomountServiceAccountToken != nil && *sa.AutomountServiceAccountToken {
		t.Error("strait-job-runner has automountServiceAccountToken=true, want false")
	}
}

// TestResourceQuota verifies the job namespace has a resource quota.
func TestResourceQuota(t *testing.T) {
	cs := getClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	quotas, err := cs.CoreV1().ResourceQuotas("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list resource quotas: %v", err)
	}
	if len(quotas.Items) == 0 {
		t.Error("no resource quota found in default namespace")
	}
}

// TestJobExecution creates a K8s Job and verifies it completes.
func TestJobExecution(t *testing.T) {
	cs := getClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	jobName := "infra-test-job"

	// Clean up any previous test job.
	_ = cs.BatchV1().Jobs("default").Delete(ctx, jobName, metav1.DeleteOptions{})
	time.Sleep(2 * time.Second)

	var backoffLimit int32
	ttl := int32(60)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
			Labels: map[string]string{
				"app":  "strait-job",
				"test": "infra-validation",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: "strait-job-runner",
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "alpine:3.19",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    *mustParseQuantity("100m"),
									corev1.ResourceMemory: *mustParseQuantity("64Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    *mustParseQuantity("500m"),
									corev1.ResourceMemory: *mustParseQuantity("128Mi"),
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := cs.BatchV1().Jobs("default").Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create test job: %v", err)
	}
	t.Cleanup(func() {
		_ = cs.BatchV1().Jobs("default").Delete(context.Background(), jobName, metav1.DeleteOptions{})
	})

	// Wait for completion.
	for range 30 {
		j, err := cs.BatchV1().Jobs("default").Get(ctx, jobName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if j.Status.Succeeded > 0 {
			t.Log("test job completed successfully")
			return
		}
		if j.Status.Failed > 0 {
			t.Fatal("test job failed")
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatal("test job timed out")
}

func mustParseQuantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}
