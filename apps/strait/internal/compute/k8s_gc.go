package compute

import (
	"context"
	"log/slog"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// K8sJobGC periodically cleans up orphaned K8s jobs that are stuck or failed.
// Safety rules:
//   - Only targets jobs with label app=strait-job.
//   - Only destroys Failed jobs or stuck-pending jobs older than maxAge.
//   - Never destroys Running or Succeeded jobs.
//   - For Pending jobs, only destroys if the pod has an unrecoverable error.
type K8sJobGC struct {
	clientset kubernetes.Interface
	namespace string
	maxAge    time.Duration // Min age before a failed/stuck job is eligible for GC.
	interval  time.Duration // How often to run the sweep.
}

// NewK8sJobGC creates a garbage collector for orphaned K8s jobs.
func NewK8sJobGC(clientset kubernetes.Interface, namespace string, maxAge, interval time.Duration) *K8sJobGC {
	if maxAge <= 0 {
		maxAge = 30 * time.Minute
	}
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &K8sJobGC{
		clientset: clientset,
		namespace: namespace,
		maxAge:    maxAge,
		interval:  interval,
	}
}

// Run starts the GC loop. Blocks until ctx is canceled.
func (gc *K8sJobGC) Run(ctx context.Context) {
	ticker := time.NewTicker(gc.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gc.Sweep(ctx)
		}
	}
}

// Sweep performs one GC pass. Exported for testing.
func (gc *K8sJobGC) Sweep(ctx context.Context) {
	jobs, err := gc.clientset.BatchV1().Jobs(gc.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=strait-job",
	})
	if err != nil {
		slog.Error("k8s gc: failed to list jobs", "error", err)
		return
	}

	var destroyed int
	for i := range jobs.Items {
		job := &jobs.Items[i]
		reason := gc.destroyReason(job)
		if reason == "" {
			continue
		}

		slog.Warn("k8s gc: destroying orphaned job",
			"job", job.Name,
			"reason", reason,
			"age", time.Since(job.CreationTimestamp.Time).Round(time.Second),
		)

		propagation := metav1.DeletePropagationBackground
		err := gc.clientset.BatchV1().Jobs(gc.namespace).Delete(ctx, job.Name, metav1.DeleteOptions{
			PropagationPolicy: &propagation,
		})
		if err != nil {
			slog.Error("k8s gc: failed to delete job", "job", job.Name, "error", err)
			continue
		}
		destroyed++
	}

	if destroyed > 0 {
		slog.Info("k8s gc: sweep complete", "destroyed", destroyed, "total_jobs", len(jobs.Items))
	}
}

// destroyReason returns the reason a job should be destroyed, or "" if it should not.
func (gc *K8sJobGC) destroyReason(job *batchv1.Job) string {
	age := time.Since(job.CreationTimestamp.Time)

	// Never destroy young jobs regardless of status.
	if age < gc.maxAge {
		return ""
	}

	// Never destroy Succeeded jobs (TTLSecondsAfterFinished handles those).
	if job.Status.Succeeded > 0 {
		return ""
	}

	// Never destroy actively Running jobs.
	if job.Status.Active > 0 {
		return ""
	}

	// Failed jobs older than maxAge: destroy.
	if job.Status.Failed > 0 {
		return "failed"
	}

	// Jobs past their deadline + buffer: destroy.
	if job.Spec.ActiveDeadlineSeconds != nil {
		deadline := time.Duration(*job.Spec.ActiveDeadlineSeconds) * time.Second
		if age > deadline+gc.maxAge {
			return "past_deadline"
		}
	}

	// Stuck pending with unrecoverable error.
	if gc.isStuckPending(job) {
		return "stuck_pending"
	}

	return ""
}

// isStuckPending checks if a job's pods are in an unrecoverable pending state.
func (gc *K8sJobGC) isStuckPending(job *batchv1.Job) bool {
	// Only check if the job has no active, succeeded, or failed pods
	// (meaning it's stuck before even starting).
	if job.Status.Active > 0 || job.Status.Succeeded > 0 || job.Status.Failed > 0 {
		return false
	}

	// Check job conditions for failure.
	for _, cond := range job.Status.Conditions {
		if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}
