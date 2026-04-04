package telemetry

import (
	"testing"
)

func TestK8sMetricsAdapter_RecordJobCreate_Success(t *testing.T) {
	m, _, _, _ := InitMetrics("test-k8s-adapter", "test")
	a := NewK8sMetricsAdapter(m)

	// Should not panic.
	a.RecordJobCreate("success", "micro", 0.5)
}

func TestK8sMetricsAdapter_RecordJobCreate_Error(t *testing.T) {
	m, _, _, _ := InitMetrics("test-k8s-adapter", "test")
	a := NewK8sMetricsAdapter(m)

	a.RecordJobCreate("error", "small-1x", 1.2)
}

func TestK8sMetricsAdapter_RecordJobWait_AllStatuses(t *testing.T) {
	m, _, _, _ := InitMetrics("test-k8s-adapter", "test")
	a := NewK8sMetricsAdapter(m)

	statuses := []string{"success", "failure", "oom", "timeout"}
	for _, s := range statuses {
		t.Run(s, func(t *testing.T) {
			a.RecordJobWait(s, 10.0)
		})
	}
}

func TestK8sMetricsAdapter_RecordPodScheduling(t *testing.T) {
	m, _, _, _ := InitMetrics("test-k8s-adapter", "test")
	a := NewK8sMetricsAdapter(m)

	a.RecordPodScheduling(2.5)
}

func TestK8sMetricsAdapter_IncJobsActive_UpDown(t *testing.T) {
	m, _, _, _ := InitMetrics("test-k8s-adapter", "test")
	a := NewK8sMetricsAdapter(m)

	a.IncJobsActive(1)
	a.IncJobsActive(1)
	a.IncJobsActive(-1)
	a.IncJobsActive(-1)
}

func TestK8sMetricsAdapter_NilMetrics(t *testing.T) {
	a := NewK8sMetricsAdapter(nil)
	if a != nil {
		t.Error("NewK8sMetricsAdapter(nil) should return nil")
	}
}
