package compute

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestGC(maxAge time.Duration) (*K8sJobGC, *fake.Clientset) {
	cs := fake.NewSimpleClientset()
	gc := NewK8sJobGC(cs, "default", maxAge, time.Minute)
	return gc, cs
}

func createTestJob(t *testing.T, cs *fake.Clientset, name string, age time.Duration, status batchv1.JobStatus, deadline *int64) {
	t.Helper()
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			Labels:            map[string]string{"app": "strait-job"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds: deadline,
		},
		Status: status,
	}
	_, err := cs.BatchV1().Jobs("default").Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create test job: %v", err)
	}
}

func jobExists(t *testing.T, cs *fake.Clientset, name string) bool {
	t.Helper()
	_, err := cs.BatchV1().Jobs("default").Get(context.Background(), name, metav1.GetOptions{})
	return err == nil
}

//nolint:modernize // Cannot use new(int64) for non-zero values.
func deadlinePtr(d int64) *int64 { return &d }

// Safety rule: never destroy young jobs.
func TestK8sGC_IgnoresYoungJobs(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)
	createTestJob(t, cs, "young-failed", 5*time.Minute, batchv1.JobStatus{Failed: 1}, nil)

	gc.Sweep(context.Background())

	if !jobExists(t, cs, "young-failed") {
		t.Error("young failed job was destroyed, should be kept (too young)")
	}
}

// Safety rule: destroy old failed jobs.
func TestK8sGC_DestroysOldFailedJob(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)
	createTestJob(t, cs, "old-failed", 45*time.Minute, batchv1.JobStatus{Failed: 1}, nil)

	gc.Sweep(context.Background())

	if jobExists(t, cs, "old-failed") {
		t.Error("old failed job was NOT destroyed, should be cleaned up")
	}
}

// Safety rule: never destroy running jobs.
func TestK8sGC_NeverDestroysRunningJob(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)
	createTestJob(t, cs, "old-running", 2*time.Hour, batchv1.JobStatus{Active: 1}, nil)

	gc.Sweep(context.Background())

	if !jobExists(t, cs, "old-running") {
		t.Error("running job was destroyed, should NEVER be destroyed")
	}
}

// Safety rule: never destroy succeeded jobs.
func TestK8sGC_NeverDestroysSucceededJob(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)
	createTestJob(t, cs, "old-succeeded", 2*time.Hour, batchv1.JobStatus{Succeeded: 1}, nil)

	gc.Sweep(context.Background())

	if !jobExists(t, cs, "old-succeeded") {
		t.Error("succeeded job was destroyed, TTLSecondsAfterFinished should handle these")
	}
}

// Safety rule: destroy jobs past their deadline.
func TestK8sGC_DestroysPastDeadline(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)
	// Job with 60s deadline, created 2 hours ago, no active pods.
	createTestJob(t, cs, "past-deadline", 2*time.Hour, batchv1.JobStatus{}, deadlinePtr(60))

	gc.Sweep(context.Background())

	if jobExists(t, cs, "past-deadline") {
		t.Error("past-deadline job was NOT destroyed")
	}
}

// Safety rule: destroy stuck pending jobs with unrecoverable error.
func TestK8sGC_DestroysStuckPending(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)
	createTestJob(t, cs, "stuck-pending", 45*time.Minute, batchv1.JobStatus{
		Conditions: []batchv1.JobCondition{
			{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "DeadlineExceeded"},
		},
	}, nil)

	gc.Sweep(context.Background())

	if jobExists(t, cs, "stuck-pending") {
		t.Error("stuck pending job was NOT destroyed")
	}
}

// Safety rule: don't destroy pending jobs that are just waiting for a node.
func TestK8sGC_IgnoresPendingWaitingForNode(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)
	// No error conditions, just waiting.
	createTestJob(t, cs, "waiting-for-node", 45*time.Minute, batchv1.JobStatus{}, nil)

	gc.Sweep(context.Background())

	if !jobExists(t, cs, "waiting-for-node") {
		t.Error("pending job waiting for node was destroyed, should be kept")
	}
}

// Safety rule: only target strait-job labeled jobs.
func TestK8sGC_IgnoresNonStraitJobs(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)
	// Create a job without the strait-job label.
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "other-job",
			Namespace:         "default",
			Labels:            map[string]string{"app": "other-app"},
			CreationTimestamp: metav1.NewTime(time.Now().Add(-2 * time.Hour)),
		},
		Status: batchv1.JobStatus{Failed: 1},
	}
	_, _ = cs.BatchV1().Jobs("default").Create(context.Background(), job, metav1.CreateOptions{})

	gc.Sweep(context.Background())

	if !jobExists(t, cs, "other-job") {
		t.Error("non-strait job was destroyed, should be ignored")
	}
}

// Empty namespace sweep.
func TestK8sGC_EmptyNamespace(t *testing.T) {
	gc, _ := newTestGC(30 * time.Minute)
	// Should not panic or error.
	gc.Sweep(context.Background())
}

// Multiple jobs, mixed statuses.
func TestK8sGC_MixedStatuses(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)

	createTestJob(t, cs, "keep-running", 2*time.Hour, batchv1.JobStatus{Active: 1}, nil)
	createTestJob(t, cs, "keep-succeeded", 2*time.Hour, batchv1.JobStatus{Succeeded: 1}, nil)
	createTestJob(t, cs, "keep-young", 5*time.Minute, batchv1.JobStatus{Failed: 1}, nil)
	createTestJob(t, cs, "destroy-failed", 45*time.Minute, batchv1.JobStatus{Failed: 1}, nil)
	createTestJob(t, cs, "destroy-deadline", 2*time.Hour, batchv1.JobStatus{}, deadlinePtr(60))

	gc.Sweep(context.Background())

	if !jobExists(t, cs, "keep-running") {
		t.Error("running job destroyed")
	}
	if !jobExists(t, cs, "keep-succeeded") {
		t.Error("succeeded job destroyed")
	}
	if !jobExists(t, cs, "keep-young") {
		t.Error("young job destroyed")
	}
	if jobExists(t, cs, "destroy-failed") {
		t.Error("old failed job NOT destroyed")
	}
	if jobExists(t, cs, "destroy-deadline") {
		t.Error("past-deadline job NOT destroyed")
	}
}

// Adversarial: nil conditions should not panic.
func TestK8sGC_Adversarial_NilConditions(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)
	createTestJob(t, cs, "nil-conditions", 45*time.Minute, batchv1.JobStatus{
		Conditions: nil,
	}, nil)

	// Should not panic.
	gc.Sweep(context.Background())
}

// Adversarial: context canceled mid-sweep.
func TestK8sGC_Adversarial_ContextCanceled(t *testing.T) {
	gc, cs := newTestGC(30 * time.Minute)
	createTestJob(t, cs, "job1", 45*time.Minute, batchv1.JobStatus{Failed: 1}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	// Should not panic. May or may not clean up depending on timing.
	gc.Sweep(ctx)
}

// Verify destroyReason returns correct strings.
func TestK8sGC_DestroyReason(t *testing.T) {
	gc, _ := newTestGC(30 * time.Minute)

	tests := []struct {
		name   string
		age    time.Duration
		status batchv1.JobStatus
		dl     *int64
		want   string
	}{
		{"young", 5 * time.Minute, batchv1.JobStatus{Failed: 1}, nil, ""},
		{"failed", 45 * time.Minute, batchv1.JobStatus{Failed: 1}, nil, "failed"},
		{"running", 2 * time.Hour, batchv1.JobStatus{Active: 1}, nil, ""},
		{"succeeded", 2 * time.Hour, batchv1.JobStatus{Succeeded: 1}, nil, ""},
		{"past_deadline", 2 * time.Hour, batchv1.JobStatus{}, deadlinePtr(60), "past_deadline"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.NewTime(time.Now().Add(-tt.age)),
				},
				Spec:   batchv1.JobSpec{ActiveDeadlineSeconds: tt.dl},
				Status: tt.status,
			}
			got := gc.destroyReason(job)
			if got != tt.want {
				t.Errorf("destroyReason() = %q, want %q", got, tt.want)
			}
		})
	}
}
