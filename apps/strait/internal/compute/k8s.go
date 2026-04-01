package compute

import (
	"context"
	"fmt"
	"io"
	"maps"
	"time"

	"github.com/google/uuid"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// K8sRuntime implements ContainerRuntime using Kubernetes Jobs via client-go.
type K8sRuntime struct {
	clientset     kubernetes.Interface
	namespace     string
	priorityClass string
}

// NewK8sRuntime creates a new Kubernetes runtime.
// If kubeconfig is empty, falls back to in-cluster config.
func NewK8sRuntime(kubeconfig, namespace, priorityClass string) (*K8sRuntime, error) {
	var cfg *rest.Config
	var err error

	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("k8s config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("k8s clientset: %w", err)
	}

	return &K8sRuntime{
		clientset:     clientset,
		namespace:     namespace,
		priorityClass: priorityClass,
	}, nil
}

// NewK8sRuntimeFromClient creates a K8sRuntime from an existing clientset (for testing).
func NewK8sRuntimeFromClient(clientset kubernetes.Interface, namespace, priorityClass string) *K8sRuntime {
	return &K8sRuntime{
		clientset:     clientset,
		namespace:     namespace,
		priorityClass: priorityClass,
	}
}

// Run creates a Kubernetes Job, waits for it to finish, and returns the result.
func (k *K8sRuntime) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	machineID, err := k.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	return k.Wait(ctx, machineID, req.TimeoutSecs)
}

// Create provisions a Kubernetes Job and returns the job name as machineID.
func (k *K8sRuntime) Create(ctx context.Context, req RunRequest) (string, error) {
	preset, err := PresetFromName(req.MachinePreset)
	if err != nil {
		return "", NewFatalError(422, "invalid machine preset", err)
	}

	jobName := "strait-" + uuid.New().String()[:8]
	requests, limits := preset.K8sResources()

	var backoffLimit int32
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: k.namespace,
			Labels:    mergeLabels(req.Labels, map[string]string{"app": "strait-job"}),
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: mergeLabels(req.Labels, map[string]string{"app": "strait-job", "job-name": jobName}),
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "job",
							Image: req.ImageURI,
							Env:   mapToEnvVars(req.Env),
							Resources: corev1.ResourceRequirements{
								Requests: requests,
								Limits:   limits,
							},
						},
					},
				},
			},
		},
	}

	if k.priorityClass != "" {
		job.Spec.Template.Spec.PriorityClassName = k.priorityClass
	}

	if req.TimeoutSecs > 0 {
		deadline := int64(req.TimeoutSecs)
		job.Spec.ActiveDeadlineSeconds = &deadline
	}

	_, err = k.clientset.BatchV1().Jobs(k.namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return "", NewFatalError(409, "job already exists", err)
		}
		if k8serrors.IsInvalid(err) {
			return "", NewFatalError(422, fmt.Sprintf("invalid job spec: %v", err), nil)
		}
		return "", NewRetryableError(500, "create k8s job", err)
	}

	return jobName, nil
}

// Wait blocks until a Kubernetes Job's pod completes and returns the result.
func (k *K8sRuntime) Wait(ctx context.Context, machineID string, timeoutSecs int) (*RunResult, error) {
	now := time.Now()
	result := &RunResult{
		MachineID: machineID,
		StartedAt: &now,
	}

	waitTimeout := 300
	if timeoutSecs > 0 {
		waitTimeout = timeoutSecs + 30
	}

	waitCtx, cancel := context.WithTimeout(ctx, time.Duration(waitTimeout)*time.Second)
	defer cancel()

	// Poll for pod completion since fake clientset doesn't support watch well.
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return result, NewTimeoutError("wait timed out", waitCtx.Err())
		case <-ticker.C:
			pod, err := k.findJobPod(waitCtx, machineID)
			if err != nil {
				continue // Pod may not exist yet.
			}

			switch pod.Status.Phase {
			case corev1.PodSucceeded, corev1.PodFailed:
				finished := time.Now()
				result.FinishedAt = &finished
				k.extractExitInfo(pod, result)

				if result.ExitCode != 0 {
					logCtx, logCancel := context.WithTimeout(ctx, 10*time.Second)
					defer logCancel()
					if logs, logErr := k.GetLogs(logCtx, machineID, 100); logErr == nil {
						result.Logs = logs
					}
				}

				return result, nil
			case corev1.PodRunning, corev1.PodUnknown:
				// Still running or unknown, continue polling.
			case corev1.PodPending:
				// Check for unrecoverable scheduling issues.
				if reason := k.podFailureReason(pod); reason != "" {
					finished := time.Now()
					result.FinishedAt = &finished
					result.ExitCode = -1
					return result, NewFatalError(422, reason, nil)
				}
			}
		}
	}
}

// Stop sends a stop signal by deleting the Job with a grace period.
func (k *K8sRuntime) Stop(ctx context.Context, machineID string) error {
	gracePeriod := int64(30)
	propagation := metav1.DeletePropagationForeground
	err := k.clientset.BatchV1().Jobs(k.namespace).Delete(ctx, machineID, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
		PropagationPolicy:  &propagation,
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ErrMachineGone
		}
		return NewRetryableError(500, "stop k8s job", err)
	}
	return nil
}

// Start always returns ErrMachineGone because Kubernetes Jobs are one-shot.
func (k *K8sRuntime) Start(_ context.Context, _ string, _ map[string]string) error {
	return ErrMachineGone
}

// Destroy deletes a Kubernetes Job and its pods immediately.
func (k *K8sRuntime) Destroy(ctx context.Context, machineID string) error {
	gracePeriod := int64(0)
	propagation := metav1.DeletePropagationBackground
	err := k.clientset.BatchV1().Jobs(k.namespace).Delete(ctx, machineID, metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
		PropagationPolicy:  &propagation,
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ErrMachineGone
		}
		return NewRetryableError(500, "destroy k8s job", err)
	}
	return nil
}

// Status returns the current state of a Kubernetes Job.
func (k *K8sRuntime) Status(ctx context.Context, machineID string) (MachineStatus, error) {
	pod, err := k.findJobPod(ctx, machineID)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return MachineStatusDestroyed, nil
		}
		// No pod found for this job.
		return MachineStatusUnknown, nil
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		return MachineStatusCreated, nil
	case corev1.PodRunning:
		return MachineStatusRunning, nil
	case corev1.PodSucceeded, corev1.PodFailed:
		return MachineStatusStopped, nil
	default:
		return MachineStatusUnknown, nil
	}
}

// GetLogs returns the last N lines of a Job pod's stdout/stderr.
func (k *K8sRuntime) GetLogs(ctx context.Context, machineID string, lines int) (string, error) {
	pod, err := k.findJobPod(ctx, machineID)
	if err != nil {
		return "", fmt.Errorf("find pod for logs: %w", err)
	}

	tailLines := int64(lines)
	logReq := k.clientset.CoreV1().Pods(k.namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		TailLines: &tailLines,
	})

	stream, err := logReq.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("open log stream: %w", err)
	}
	defer stream.Close()

	body, err := io.ReadAll(io.LimitReader(stream, 1<<20)) // Cap at 1MB.
	if err != nil {
		return "", fmt.Errorf("read logs: %w", err)
	}

	return string(body), nil
}

// findJobPod returns the first pod belonging to the given job.
func (k *K8sRuntime) findJobPod(ctx context.Context, jobName string) (*corev1.Pod, error) {
	pods, err := k.clientset.CoreV1().Pods(k.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
		Limit:         1,
	})
	if err != nil {
		return nil, err
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods found for job %s", jobName)
	}
	return &pods.Items[0], nil
}

// extractExitInfo populates RunResult from pod container statuses.
func (k *K8sRuntime) extractExitInfo(pod *corev1.Pod, result *RunResult) {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name != "job" {
			continue
		}
		if cs.State.Terminated != nil {
			result.ExitCode = int(cs.State.Terminated.ExitCode)
			if cs.State.Terminated.Signal > 0 {
				result.ExitSignal = fmt.Sprintf("signal-%d", cs.State.Terminated.Signal)
			}
			if cs.State.Terminated.Reason == "OOMKilled" {
				result.OOMKilled = true
			}
		}
	}
}

// podFailureReason returns a reason string if the pod is in an unrecoverable state.
func (k *K8sRuntime) podFailureReason(pod *corev1.Pod) string {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			if cond.Reason == "Unschedulable" {
				return fmt.Sprintf("pod unschedulable: %s", cond.Message)
			}
		}
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			switch cs.State.Waiting.Reason {
			case "ImagePullBackOff", "ErrImagePull", "InvalidImageName":
				return fmt.Sprintf("image error: %s: %s", cs.State.Waiting.Reason, cs.State.Waiting.Message)
			}
		}
	}
	return ""
}

// mapToEnvVars converts a map to Kubernetes EnvVar slice.
func mapToEnvVars(env map[string]string) []corev1.EnvVar {
	if len(env) == 0 {
		return nil
	}
	vars := make([]corev1.EnvVar, 0, len(env))
	for k, v := range env {
		vars = append(vars, corev1.EnvVar{Name: k, Value: v})
	}
	return vars
}

// mergeLabels merges base labels with overrides.
func mergeLabels(base, overrides map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(overrides))
	maps.Copy(merged, base)
	maps.Copy(merged, overrides)
	return merged
}

// Ensure K8sRuntime implements ContainerRuntime.
var _ ContainerRuntime = (*K8sRuntime)(nil)
