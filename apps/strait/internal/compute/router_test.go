package compute

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// mockRuntime is a simple mock for router testing.
type mockRuntime struct {
	createFn  func(ctx context.Context, req RunRequest) (string, error)
	waitFn    func(ctx context.Context, id string, timeout int) (*RunResult, error)
	stopFn    func(ctx context.Context, id string) error
	destroyFn func(ctx context.Context, id string) error
	statusFn  func(ctx context.Context, id string) (MachineStatus, error)
	getLogsFn func(ctx context.Context, id string, lines int) (string, error)
}

func (m *mockRuntime) Run(ctx context.Context, req RunRequest) (*RunResult, error) {
	id, err := m.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	return m.Wait(ctx, id, req.TimeoutSecs)
}

func (m *mockRuntime) Create(ctx context.Context, req RunRequest) (string, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return "mock-id", nil
}

func (m *mockRuntime) Wait(ctx context.Context, id string, timeout int) (*RunResult, error) {
	if m.waitFn != nil {
		return m.waitFn(ctx, id, timeout)
	}
	return &RunResult{MachineID: id, ExitCode: 0}, nil
}

func (m *mockRuntime) Stop(ctx context.Context, id string) error {
	if m.stopFn != nil {
		return m.stopFn(ctx, id)
	}
	return nil
}

func (m *mockRuntime) Start(_ context.Context, _ string, _ map[string]string) error {
	return ErrMachineGone
}

func (m *mockRuntime) Destroy(ctx context.Context, id string) error {
	if m.destroyFn != nil {
		return m.destroyFn(ctx, id)
	}
	return nil
}

func (m *mockRuntime) Status(ctx context.Context, id string) (MachineStatus, error) {
	if m.statusFn != nil {
		return m.statusFn(ctx, id)
	}
	return MachineStatusStopped, nil
}

func (m *mockRuntime) GetLogs(ctx context.Context, id string, lines int) (string, error) {
	if m.getLogsFn != nil {
		return m.getLogsFn(ctx, id, lines)
	}
	return "mock logs", nil
}

func TestRuntimeRouter_PrimarySuccess(t *testing.T) {
	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			return "primary-id", nil
		},
	}
	fallback := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			t.Error("fallback should not be called")
			return "", nil
		},
	}

	router := NewRuntimeRouter(primary, fallback)
	id, err := router.Create(context.Background(), RunRequest{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if id != "primary-id" {
		t.Errorf("Create() = %q, want primary-id", id)
	}
}

func TestRuntimeRouter_FallbackOnRetryable(t *testing.T) {
	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			return "", NewRetryableError(500, "primary down", nil)
		},
	}
	fallback := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			return "fallback-id", nil
		},
	}

	router := NewRuntimeRouter(primary, fallback)
	id, err := router.Create(context.Background(), RunRequest{})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if id != "fallback-id" {
		t.Errorf("Create() = %q, want fallback-id", id)
	}
}

func TestRuntimeRouter_NoFallbackOnFatal(t *testing.T) {
	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			return "", NewFatalError(422, "bad config", nil)
		},
	}
	fallback := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			t.Error("fallback should not be called on fatal error")
			return "", nil
		},
	}

	router := NewRuntimeRouter(primary, fallback)
	_, err := router.Create(context.Background(), RunRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsFatal(err) {
		t.Errorf("expected fatal error, got: %v", err)
	}
}

func TestRuntimeRouter_NilFallback(t *testing.T) {
	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			return "", NewRetryableError(500, "primary down", nil)
		},
	}

	router := NewRuntimeRouter(primary, nil)
	_, err := router.Create(context.Background(), RunRequest{})
	if err == nil {
		t.Fatal("expected error with nil fallback")
	}
	if !IsRetryable(err) {
		t.Errorf("expected retryable error, got: %v", err)
	}
}

func TestRuntimeRouter_WaitRoutesToCorrectRuntime(t *testing.T) {
	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			return "p-id", nil
		},
		waitFn: func(_ context.Context, id string, _ int) (*RunResult, error) {
			return &RunResult{MachineID: id, ExitCode: 0}, nil
		},
	}
	fallback := &mockRuntime{
		waitFn: func(_ context.Context, _ string, _ int) (*RunResult, error) {
			t.Error("fallback Wait should not be called for primary-created machine")
			return nil, errors.New("wrong runtime")
		},
	}

	router := NewRuntimeRouter(primary, fallback)
	ctx := context.Background()
	id, _ := router.Create(ctx, RunRequest{})
	result, err := router.Wait(ctx, id, 30)
	if err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if result.MachineID != "p-id" {
		t.Errorf("Wait().MachineID = %q, want p-id", result.MachineID)
	}
}

func TestRuntimeRouter_DestroyRoutesToCorrectRuntime(t *testing.T) {
	var primaryDestroyed bool
	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) { return "p-id", nil },
		destroyFn: func(_ context.Context, _ string) error {
			primaryDestroyed = true
			return nil
		},
	}
	fallback := &mockRuntime{
		destroyFn: func(_ context.Context, _ string) error {
			t.Error("fallback Destroy should not be called")
			return nil
		},
	}

	router := NewRuntimeRouter(primary, fallback)
	ctx := context.Background()
	id, _ := router.Create(ctx, RunRequest{})
	_ = router.Destroy(ctx, id)

	if !primaryDestroyed {
		t.Error("primary.Destroy was not called")
	}
}

func TestRuntimeRouter_FallbackCreatedMachineRoutesToFallback(t *testing.T) {
	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			return "", NewRetryableError(500, "down", nil)
		},
	}

	var fallbackWaitCalled bool
	fallback := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) { return "f-id", nil },
		waitFn: func(_ context.Context, _ string, _ int) (*RunResult, error) {
			fallbackWaitCalled = true
			return &RunResult{ExitCode: 0}, nil
		},
	}

	router := NewRuntimeRouter(primary, fallback)
	ctx := context.Background()
	id, _ := router.Create(ctx, RunRequest{})
	_, _ = router.Wait(ctx, id, 30)

	if !fallbackWaitCalled {
		t.Error("fallback.Wait was not called for fallback-created machine")
	}
}

func TestRuntimeRouter_GetLogsRoutesToCorrectRuntime(t *testing.T) {
	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) { return "p-id", nil },
		getLogsFn: func(_ context.Context, _ string, _ int) (string, error) {
			return "primary logs", nil
		},
	}
	fallback := &mockRuntime{}

	router := NewRuntimeRouter(primary, fallback)
	ctx := context.Background()
	id, _ := router.Create(ctx, RunRequest{})
	logs, _ := router.GetLogs(ctx, id, 10)
	if logs != "primary logs" {
		t.Errorf("GetLogs() = %q, want primary logs", logs)
	}
}

func TestRuntimeRouter_StatusRoutesToCorrectRuntime(t *testing.T) {
	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) { return "p-id", nil },
		statusFn: func(_ context.Context, _ string) (MachineStatus, error) {
			return MachineStatusRunning, nil
		},
	}
	fallback := &mockRuntime{}

	router := NewRuntimeRouter(primary, fallback)
	ctx := context.Background()
	id, _ := router.Create(ctx, RunRequest{})
	status, _ := router.Status(ctx, id)
	if status != MachineStatusRunning {
		t.Errorf("Status() = %v, want Running", status)
	}
}

func TestRuntimeRouter_ConcurrentCreates(t *testing.T) {
	var primaryCount, fallbackCount int64
	var mu sync.Mutex

	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			mu.Lock()
			primaryCount++
			id := primaryCount
			mu.Unlock()
			return "p-" + string(rune('0'+id)), nil
		},
	}
	fallback := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) {
			mu.Lock()
			fallbackCount++
			mu.Unlock()
			return "f-id", nil
		},
	}

	router := NewRuntimeRouter(primary, fallback)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, _ = router.Create(context.Background(), RunRequest{})
		})
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if primaryCount != 20 {
		t.Errorf("primary called %d times, want 20", primaryCount)
	}
	if fallbackCount != 0 {
		t.Errorf("fallback called %d times, want 0", fallbackCount)
	}
}

func TestRuntimeRouter_DestroyCleanupOwnerMap(t *testing.T) {
	primary := &mockRuntime{
		createFn: func(_ context.Context, _ RunRequest) (string, error) { return "p-id", nil },
	}

	router := NewRuntimeRouter(primary, nil)
	ctx := context.Background()
	id, _ := router.Create(ctx, RunRequest{})
	_ = router.Destroy(ctx, id)

	// After destroy, owner should be cleaned up. Next call should go to primary (default).
	status, _ := router.Status(ctx, id)
	_ = status // Just verify no panic.
}
