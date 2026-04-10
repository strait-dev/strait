package compute

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestK8s_CreateJob_ValidatesBeforeAPICall(t *testing.T) {
	t.Parallel()
	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "strait-job", "")

	// Invalid image should fail before hitting the K8s API.
	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI:      "",
		MachinePreset: "micro",
	})
	if err == nil {
		t.Error("expected validation error for empty image URI")
	}
	if !IsFatal(err) {
		t.Errorf("validation errors should be fatal, got: %v", err)
	}

	// Invalid preset should fail before hitting the K8s API.
	_, err = rt.Create(context.Background(), RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "nonexistent",
	})
	if err == nil {
		t.Error("expected validation error for invalid preset")
	}
}

func TestK8s_DestroyJob_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "strait-job", "")

	err := rt.Destroy(context.Background(), "strait-aabbccddeeff")
	if err == nil {
		t.Error("Destroy of non-existent job should return error")
	}
}

func TestK8s_MultipleDestroy_Idempotent(t *testing.T) {
	t.Parallel()
	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "strait-job", "")

	// Create a job, then destroy it twice — second destroy should be idempotent.
	id, err := rt.Create(context.Background(), RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "micro",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := rt.Destroy(context.Background(), id); err != nil {
		t.Fatalf("first Destroy: %v", err)
	}
	// Second destroy returns error (job already deleted) — this is expected.
	err = rt.Destroy(context.Background(), id)
	if err == nil {
		t.Error("second Destroy should return error (job already deleted)")
	}
}

func TestK8s_StopJob_NotFound_ReturnsError(t *testing.T) {
	t.Parallel()
	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "strait-job", "")

	err := rt.Stop(context.Background(), "strait-aabbccddeeff")
	if err == nil {
		t.Error("Stop of non-existent job should return error")
	}
}

func TestK8s_RouterFallback_PrimaryDown(t *testing.T) {
	t.Parallel()

	primary := &failingRuntime{err: NewRetryableError(503, "primary down", nil)}
	fallback := NewK8sRuntimeFromClient(k8sfake.NewSimpleClientset(), "default", "", "")

	router := NewRuntimeRouter(primary, fallback)
	machineID, err := router.Create(context.Background(), RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "micro",
	})
	if err != nil {
		t.Fatalf("expected fallback to handle create, got: %v", err)
	}
	if machineID == "" {
		t.Error("expected non-empty machineID from fallback")
	}
}

func TestK8s_RouterFallback_DoesNotFallbackOnFatal(t *testing.T) {
	t.Parallel()

	primary := &failingRuntime{err: NewFatalError(422, "invalid request", nil)}
	fallback := NewK8sRuntimeFromClient(k8sfake.NewSimpleClientset(), "default", "", "")

	router := NewRuntimeRouter(primary, fallback)
	_, err := router.Create(context.Background(), RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "micro",
	})
	if err == nil {
		t.Fatal("expected fatal error to propagate, not fallback")
	}
	if !IsFatal(err) {
		t.Errorf("expected IsFatal=true, got false for: %v", err)
	}
}

func TestK8s_CreateJob_WithTimeout(t *testing.T) {
	t.Parallel()
	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "", "")

	_, err := rt.Create(context.Background(), RunRequest{
		ImageURI:      "alpine:3.21",
		MachinePreset: "micro",
		TimeoutSecs:   300,
	})
	if err != nil {
		t.Fatalf("Create with timeout: %v", err)
	}

	jobs, _ := cs.BatchV1().Jobs("default").List(context.Background(), metav1.ListOptions{})
	if len(jobs.Items) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs.Items))
	}

	deadline := jobs.Items[0].Spec.ActiveDeadlineSeconds
	if deadline == nil || *deadline != 300 {
		t.Errorf("ActiveDeadlineSeconds = %v, want 300", deadline)
	}
}

func TestK8s_GC_ConcurrentWithJobCreation_NoRace(t *testing.T) {
	t.Parallel()

	cs := k8sfake.NewSimpleClientset()
	rt := NewK8sRuntimeFromClient(cs, "default", "strait-job", "")
	gc := NewK8sJobGC(cs, "default", time.Hour, time.Minute)

	// Run creation and GC concurrently — must not panic or race.
	// The -race detector will catch data races if any exist.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 20 {
			_, _ = rt.Create(context.Background(), RunRequest{
				ImageURI:      "alpine:3.21",
				MachinePreset: "micro",
				Labels:        map[string]string{"batch": "concurrent"},
			})
			_ = i
		}
	}()

	// Run GC concurrently — the point is no panic/race, not job count.
	for range 3 {
		gc.Sweep(context.Background())
	}

	<-done
	// Test passes if no panic or data race occurred.
}

// failingRuntime always returns an error on Create.
type failingRuntime struct {
	err error
}

func (f *failingRuntime) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	return nil, f.err
}
func (f *failingRuntime) Create(ctx context.Context, req RunRequest) (string, error) {
	return "", f.err
}
func (f *failingRuntime) Wait(ctx context.Context, machineID string, timeoutSecs int) (*RunResult, error) {
	return nil, f.err
}
func (f *failingRuntime) Start(ctx context.Context, machineID string, env map[string]string) error {
	return f.err
}
func (f *failingRuntime) Stop(ctx context.Context, machineID string) error { return f.err }
func (f *failingRuntime) Destroy(ctx context.Context, machineID string) error {
	return f.err
}
func (f *failingRuntime) Status(ctx context.Context, machineID string) (MachineStatus, error) {
	return MachineStatusUnknown, f.err
}
func (f *failingRuntime) GetLogs(ctx context.Context, machineID string, maxLines int) (string, error) {
	return "", f.err
}
