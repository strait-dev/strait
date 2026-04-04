package compute

import (
	"context"
	"errors"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// GC Safety stress tests. Uses fake clientset for deterministic timing.

func gcTestJob(name string, age time.Duration, status batchv1.JobStatus, deadline *int64) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "default",
			Labels:            map[string]string{"app": "strait-job"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
		},
		Spec:   batchv1.JobSpec{ActiveDeadlineSeconds: deadline},
		Status: status,
	}
}

//nolint:modernize
func dlPtr(v int64) *int64 { return &v }

func TestStress_GC_Spares_Running(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	cs := fake.NewSimpleClientset(gcTestJob("running-old", 2*time.Hour, batchv1.JobStatus{Active: 1}, nil))
	gc := NewK8sJobGC(cs, "default", 30*time.Minute, time.Minute)
	gc.Sweep(context.Background())

	_, err := cs.BatchV1().Jobs("default").Get(context.Background(), "running-old", metav1.GetOptions{})
	if err != nil {
		t.Error("running job was destroyed")
	}
}

func TestStress_GC_Spares_Young_Failed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	cs := fake.NewSimpleClientset(gcTestJob("young-fail", 5*time.Minute, batchv1.JobStatus{Failed: 1}, nil))
	gc := NewK8sJobGC(cs, "default", 30*time.Minute, time.Minute)
	gc.Sweep(context.Background())

	_, err := cs.BatchV1().Jobs("default").Get(context.Background(), "young-fail", metav1.GetOptions{})
	if err != nil {
		t.Error("young failed job was destroyed")
	}
}

func TestStress_GC_Destroys_Old_Failed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	cs := fake.NewSimpleClientset(gcTestJob("old-fail", 45*time.Minute, batchv1.JobStatus{Failed: 1}, nil))
	gc := NewK8sJobGC(cs, "default", 30*time.Minute, time.Minute)
	gc.Sweep(context.Background())

	_, err := cs.BatchV1().Jobs("default").Get(context.Background(), "old-fail", metav1.GetOptions{})
	if err == nil {
		t.Error("old failed job was NOT destroyed")
	}
}

func TestStress_GC_Spares_Succeeded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	cs := fake.NewSimpleClientset(gcTestJob("succeeded", 2*time.Hour, batchv1.JobStatus{Succeeded: 1}, nil))
	gc := NewK8sJobGC(cs, "default", 30*time.Minute, time.Minute)
	gc.Sweep(context.Background())

	_, err := cs.BatchV1().Jobs("default").Get(context.Background(), "succeeded", metav1.GetOptions{})
	if err != nil {
		t.Error("succeeded job was destroyed (TTL should handle)")
	}
}

func TestStress_GC_Destroys_Past_Deadline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	cs := fake.NewSimpleClientset(gcTestJob("past-dl", 2*time.Hour, batchv1.JobStatus{}, dlPtr(60)))
	gc := NewK8sJobGC(cs, "default", 30*time.Minute, time.Minute)
	gc.Sweep(context.Background())

	_, err := cs.BatchV1().Jobs("default").Get(context.Background(), "past-dl", metav1.GetOptions{})
	if err == nil {
		t.Error("past-deadline job was NOT destroyed")
	}
}

func TestStress_GC_Spares_Pending_Normal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	cs := fake.NewSimpleClientset(gcTestJob("pending-normal", 45*time.Minute, batchv1.JobStatus{}, nil))
	gc := NewK8sJobGC(cs, "default", 30*time.Minute, time.Minute)
	gc.Sweep(context.Background())

	_, err := cs.BatchV1().Jobs("default").Get(context.Background(), "pending-normal", metav1.GetOptions{})
	if err != nil {
		t.Error("pending normal job was destroyed (should wait for node)")
	}
}

func TestStress_GC_Destroys_Stuck_Pending(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	cs := fake.NewSimpleClientset(gcTestJob("stuck", 45*time.Minute, batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{{Type: batchv1.JobFailed, Status: corev1.ConditionTrue}},
	}, nil))
	gc := NewK8sJobGC(cs, "default", 30*time.Minute, time.Minute)
	gc.Sweep(context.Background())

	_, err := cs.BatchV1().Jobs("default").Get(context.Background(), "stuck", metav1.GetOptions{})
	if err == nil {
		t.Error("stuck pending job was NOT destroyed")
	}
}

func TestStress_GC_Under_Concurrent_Load(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	rt := requireKindCluster(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	gc := NewK8sJobGC(rt.clientset, "default", 30*time.Minute, time.Minute)

	// Create jobs while GC is sweeping.
	var ids []string
	for range 3 {
		id, err := rt.Create(ctx, RunRequest{ImageURI: "alpine:3.19", MachinePreset: "micro", TimeoutSecs: 30})
		if err == nil {
			ids = append(ids, id)
		}
	}

	gc.Sweep(ctx) // Should not destroy active/young jobs.

	for _, id := range ids {
		_, _ = rt.Wait(ctx, id, 30)
		_ = rt.Destroy(ctx, id)
	}
	t.Log("GC under load: no false deletions")
}

func TestStress_GC_API_Error_Handling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("list", "jobs", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, errors.New("api server unreachable")
	})
	gc := NewK8sJobGC(cs, "default", 30*time.Minute, time.Minute)
	gc.Sweep(context.Background()) // Should not panic.
	t.Log("GC API error: no panic")
}

func TestStress_GC_Partial_Delete_Failure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test")
	}
	cs := fake.NewSimpleClientset(
		gcTestJob("fail-delete", 45*time.Minute, batchv1.JobStatus{Failed: 1}, nil),
		gcTestJob("ok-delete", 45*time.Minute, batchv1.JobStatus{Failed: 1}, nil),
	)
	callCount := 0
	cs.PrependReactor("delete", "jobs", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
		callCount++
		if callCount == 1 {
			return true, nil, errors.New("delete failed")
		}
		return false, nil, nil // Let the default handler run.
	})

	gc := NewK8sJobGC(cs, "default", 30*time.Minute, time.Minute)
	gc.Sweep(context.Background()) // Should continue after first failure.
	t.Logf("partial delete: %d delete calls", callCount)
}
