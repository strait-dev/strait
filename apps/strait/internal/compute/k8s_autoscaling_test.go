package compute

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestK8s_ConcurrentJobCreation_ScalePressure(t *testing.T) {
	t.Parallel()

	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "strait-job")

	const n = 50
	var succeeded atomic.Int64
	var wg sync.WaitGroup
	wg.Add(n)

	for i := range n {
		go func(idx int) {
			defer wg.Done()
			_, err := rt.Create(context.Background(), RunRequest{
				ImageURI:      "alpine:3.21",
				MachinePreset: "micro",
				Labels:        map[string]string{"batch": "scale-test"},
			})
			if err == nil {
				succeeded.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if succeeded.Load() != n {
		t.Errorf("only %d/%d jobs succeeded", succeeded.Load(), n)
	}

	jobs, err := cs.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs.Items) != n {
		t.Errorf("expected %d jobs, got %d", n, len(jobs.Items))
	}
}

func TestK8s_JobCreation_AllPresetsUnderLoad(t *testing.T) {
	t.Parallel()

	presets := []string{"micro", "small-1x", "small-2x", "medium-1x", "medium-2x", "large-1x", "large-2x"}
	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "strait-job")

	var wg sync.WaitGroup
	wg.Add(len(presets))

	for _, preset := range presets {
		go func(p string) {
			defer wg.Done()
			_, err := rt.Create(context.Background(), RunRequest{
				ImageURI:      "alpine:3.21",
				MachinePreset: p,
			})
			if err != nil {
				t.Errorf("preset %s failed: %v", p, err)
			}
		}(preset)
	}

	wg.Wait()

	jobs, _ := cs.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != len(presets) {
		t.Errorf("expected %d jobs (one per preset), got %d", len(presets), len(jobs.Items))
	}
}

func TestK8s_GC_DoesNotDeleteRunningJobs(t *testing.T) {
	t.Parallel()

	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "strait-job")

	// Create 5 jobs.
	for i := range 5 {
		_, err := rt.Create(context.Background(), RunRequest{
			ImageURI:      "alpine:3.21",
			MachinePreset: "micro",
		})
		if err != nil {
			t.Fatalf("create job %d: %v", i, err)
		}
	}

	// Mark all jobs as Running (active = 1).
	jobs, _ := cs.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{})
	now := metav1.Now()
	for i := range jobs.Items {
		jobs.Items[i].Status.Active = 1
		jobs.Items[i].Status.StartTime = &now
		_, _ = cs.BatchV1().Jobs("default").UpdateStatus(context.Background(), &jobs.Items[i], metav1.UpdateOptions{})
	}

	// Run GC — Running jobs must NOT be deleted regardless of age.
	gc := NewK8sJobGC(cs, "default", 0, time.Minute) // maxAge=0 means everything is old enough
	gc.Sweep(context.Background())

	remaining, _ := cs.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{
		LabelSelector: "app=strait-job",
	})
	if len(remaining.Items) != 5 {
		t.Errorf("GC deleted Running jobs: expected 5, got %d", len(remaining.Items))
	}
}
