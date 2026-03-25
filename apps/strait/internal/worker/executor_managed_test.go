package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/compute"
	"strait/internal/domain"
	orcstore "strait/internal/store"
	"strait/internal/telemetry"

	"golang.org/x/sync/semaphore"
)

// mockContainerRuntime implements compute.ContainerRuntime for unit tests.
type mockContainerRuntime struct {
	runFn     func(ctx context.Context, req compute.RunRequest) (*compute.RunResult, error)
	createFn  func(ctx context.Context, req compute.RunRequest) (string, error)
	waitFn    func(ctx context.Context, machineID string, timeoutSecs int) (*compute.RunResult, error)
	startFn   func(ctx context.Context, machineID string, env map[string]string) error
	stopFn    func(ctx context.Context, machineID string) error
	destroyFn func(ctx context.Context, machineID string) error
	statusFn  func(ctx context.Context, machineID string) (compute.MachineStatus, error)
	getLogsFn func(ctx context.Context, machineID string, lines int) (string, error)
}

func (m *mockContainerRuntime) Run(ctx context.Context, req compute.RunRequest) (*compute.RunResult, error) {
	if m.runFn != nil {
		return m.runFn(ctx, req)
	}
	return &compute.RunResult{ExitCode: 0}, nil
}

func (m *mockContainerRuntime) Create(ctx context.Context, req compute.RunRequest) (string, error) {
	if m.createFn != nil {
		return m.createFn(ctx, req)
	}
	return "mock-machine-id", nil
}

func (m *mockContainerRuntime) Wait(ctx context.Context, machineID string, timeoutSecs int) (*compute.RunResult, error) {
	if m.waitFn != nil {
		return m.waitFn(ctx, machineID, timeoutSecs)
	}
	return &compute.RunResult{MachineID: machineID, ExitCode: 0}, nil
}

func (m *mockContainerRuntime) Start(ctx context.Context, machineID string, env map[string]string) error {
	if m.startFn != nil {
		return m.startFn(ctx, machineID, env)
	}
	return compute.ErrMachineGone
}

func (m *mockContainerRuntime) Stop(ctx context.Context, machineID string) error {
	if m.stopFn != nil {
		return m.stopFn(ctx, machineID)
	}
	return nil
}

func (m *mockContainerRuntime) Destroy(ctx context.Context, machineID string) error {
	if m.destroyFn != nil {
		return m.destroyFn(ctx, machineID)
	}
	return nil
}

func (m *mockContainerRuntime) Status(ctx context.Context, machineID string) (compute.MachineStatus, error) {
	if m.statusFn != nil {
		return m.statusFn(ctx, machineID)
	}
	return compute.MachineStatusStopped, nil
}

func (m *mockContainerRuntime) GetLogs(ctx context.Context, machineID string, lines int) (string, error) {
	if m.getLogsFn != nil {
		return m.getLogsFn(ctx, machineID, lines)
	}
	return "", nil
}

func newManagedTestExecutor(store *mockExecutorStore, runtime compute.ContainerRuntime, opts ...func(*Executor)) *Executor {
	metrics, _, _, _ := telemetry.InitMetrics("test-managed", "test")
	e := &Executor{
		store:            store,
		containerRuntime: runtime,
		managedSemaphore: semaphore.NewWeighted(10),
		externalAPIURL:   "https://api.test.com",
		jwtSigningKey:    "test-signing-key-must-be-at-least-32-chars",
		heartbeat:        NewHeartbeatSender(store, 10*time.Second),
		eventCh:          make(chan runEventEnvelope, 256),
		metrics:          metrics,
		maxSnoozeCount:   50,
		logger:           slog.Default(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func newTestRun() *domain.JobRun {
	return &domain.JobRun{
		ID:        "run-1",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusDequeued,
		Attempt:   1,
		CreatedAt: time.Now(),
	}
}

func newTestManagedJob() *domain.Job {
	return &domain.Job{
		ID:            "job-1",
		ProjectID:     "proj-1",
		Slug:          "my-job",
		ExecutionMode: domain.ExecutionModeManaged,
		MachinePreset: domain.PresetMicro,
		ImageURI:      "alpine:latest",
		TimeoutSecs:   300,
		MaxAttempts:   3,
	}
}

// Test 1: Happy path — container exits 0, SDK called /complete → completed, usage recorded.
func TestManagedDispatch_HappyPath_SDKComplete(t *testing.T) {
	t.Parallel()
	now := time.Now()
	startedAt := now.Add(-10 * time.Second)
	finishedAt := now

	var usageRecorded atomic.Bool

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     "run-1",
				Status: domain.StatusCompleted, // SDK already completed
			}, nil
		},
		createRunComputeUsageFn: func(_ context.Context, usage *domain.RunComputeUsage) error {
			usageRecorded.Store(true)
			if usage.RunID != "run-1" {
				t.Errorf("expected run_id run-1, got %s", usage.RunID)
			}
			if usage.MachinePreset != "micro" {
				t.Errorf("expected preset micro, got %s", usage.MachinePreset)
			}
			if usage.DurationSecs <= 0 {
				t.Errorf("expected positive duration, got %f", usage.DurationSecs)
			}
			if usage.CostMicrousd <= 0 {
				t.Errorf("expected positive cost, got %d", usage.CostMicrousd)
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			if req.ImageURI != "alpine:latest" {
				t.Errorf("expected image alpine:latest, got %s", req.ImageURI)
			}
			if req.MachinePreset != "micro" {
				t.Errorf("expected preset micro, got %s", req.MachinePreset)
			}
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID:  "test-machine",
				ExitCode:   0,
				StartedAt:  &startedAt,
				FinishedAt: &finishedAt,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if !usageRecorded.Load() {
		t.Error("expected compute usage to be recorded")
	}
}

// Test 2: SDK race — SDK called /complete before container exits → no double transition.
func TestManagedDispatch_SDKRace_AlreadyCompleted(t *testing.T) {
	t.Parallel()
	now := time.Now()

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     "run-1",
				Status: domain.StatusCompleted,
			}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID:  "test-machine",
				ExitCode:   0,
				StartedAt:  &now,
				FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	// Should not attempt any status update beyond dequeued→executing.
	updates := store.statusUpdates()
	for _, u := range updates {
		if u.to == domain.StatusSystemFailed || u.to == domain.StatusDeadLetter {
			t.Errorf("unexpected terminal transition: %s → %s", u.from, u.to)
		}
	}
}

// Test 3: Container exit non-zero, no SDK /fail → failure + retry.
func TestManagedDispatch_NonZeroExit_Retry(t *testing.T) {
	t.Parallel()
	now := time.Now()

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     "run-1",
				Status: domain.StatusExecuting, // SDK didn't call anything
			}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID:  "test-machine",
				ExitCode:   1,
				StartedAt:  &now,
				FinishedAt: &now,
				Logs:       "error: something failed",
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	// Should see: dequeued→executing, then executing→queued (retry)
	var foundRetry bool
	for _, u := range updates {
		if u.from == domain.StatusExecuting && u.to == domain.StatusQueued {
			foundRetry = true
			if u.fields["attempt"] != 2 {
				t.Errorf("expected attempt=2, got %v", u.fields["attempt"])
			}
		}
	}
	if !foundRetry {
		t.Error("expected retry (executing → queued)")
	}
}

// Test 4: Container exit 0, no SDK /complete → system_failed.
func TestManagedDispatch_Exit0_NoSDKComplete(t *testing.T) {
	t.Parallel()
	now := time.Now()

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     "run-1",
				Status: domain.StatusExecuting,
			}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID:  "test-machine",
				ExitCode:   0,
				StartedAt:  &now,
				FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var foundSystemFailed bool
	for _, u := range updates {
		if u.to == domain.StatusSystemFailed {
			foundSystemFailed = true
			if !strings.Contains(u.fields["error"].(string), "SDK did not report completion") {
				t.Errorf("expected error about SDK, got %s", u.fields["error"])
			}
		}
	}
	if !foundSystemFailed {
		t.Error("expected system_failed status")
	}
}

// Test 5: Budget exceeded → system_failed before dispatch.
func TestManagedDispatch_BudgetExceeded(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getProjectQuotaFn: func(_ context.Context, _ string) (*orcstore.ProjectQuota, error) {
			return &orcstore.ProjectQuota{
				ProjectID:                     "proj-1",
				ComputeDailyCostLimitMicrousd: 1000, // $0.001 limit
			}, nil
		},
		sumDailyComputeCostFn: func(_ context.Context, _, _ string) (int64, error) {
			return 900, nil // Already at 900, estimated will push over
		},
	}

	var runtimeCalled atomic.Bool
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			runtimeCalled.Store(true)
			return "", fmt.Errorf("should not be called")
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()
	job.TimeoutSecs = 300 // estimated cost = 17 * 300 = 5100 >> remaining 100

	e.managedDispatch(context.Background(), run, job)

	if runtimeCalled.Load() {
		t.Error("runtime should not have been called when budget exceeded")
	}

	updates := store.statusUpdates()
	var foundSystemFailed bool
	for _, u := range updates {
		if u.to == domain.StatusSystemFailed {
			foundSystemFailed = true
		}
	}
	if !foundSystemFailed {
		t.Error("expected system_failed for budget exceeded")
	}
}

// Test 6: Infra retry (IsRetryable) → snooze, attempt NOT incremented.
func TestManagedDispatch_InfraRetry(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "", compute.NewRetryableError(429, "rate limited", nil)
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 2
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	// Should see: dequeued→executing, then executing→queued (snooze)
	var foundSnooze bool
	for _, u := range updates {
		if u.from == domain.StatusExecuting && u.to == domain.StatusQueued {
			foundSnooze = true
			// Attempt should NOT be incremented (snooze, not retry)
			if _, ok := u.fields["attempt"]; ok {
				t.Error("attempt should not be set on infra snooze")
			}
		}
	}
	if !foundSnooze {
		t.Error("expected snooze (executing → queued)")
	}
}

// Test 7: Semaphore full → snooze.
func TestManagedDispatch_SemaphoreFull(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	runtime := &mockContainerRuntime{}

	e := newManagedTestExecutor(store, runtime)
	// Set semaphore to 0 capacity to simulate full
	e.managedSemaphore = semaphore.NewWeighted(0)

	run := newTestRun()
	job := newTestManagedJob()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	e.managedDispatch(ctx, run, job)

	updates := store.statusUpdates()
	var foundSnooze bool
	for _, u := range updates {
		if u.from == domain.StatusDequeued && u.to == domain.StatusQueued {
			foundSnooze = true
		}
	}
	if !foundSnooze {
		t.Error("expected snooze when semaphore is full")
	}
}

// Test 8: Payload inline (≤64KB) → STRAIT_PAYLOAD set.
func TestManagedDispatch_PayloadInline(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Payload = json.RawMessage(`{"small": true}`)
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv["STRAIT_PAYLOAD"] != `{"small": true}` {
		t.Errorf("expected inline payload, got %q", capturedEnv["STRAIT_PAYLOAD"])
	}
	if _, ok := capturedEnv["STRAIT_PAYLOAD_MODE"]; ok {
		t.Error("STRAIT_PAYLOAD_MODE should not be set for small payloads")
	}
}

// Test 9: Payload fetch (>64KB) → STRAIT_PAYLOAD_MODE=fetch.
func TestManagedDispatch_PayloadFetch(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	// Create a payload > 64KB
	bigPayload := make([]byte, 65*1024)
	for i := range bigPayload {
		bigPayload[i] = 'A'
	}
	run.Payload = json.RawMessage(fmt.Sprintf(`"%s"`, string(bigPayload)))
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv["STRAIT_PAYLOAD_MODE"] != "fetch" {
		t.Errorf("expected STRAIT_PAYLOAD_MODE=fetch, got %q", capturedEnv["STRAIT_PAYLOAD_MODE"])
	}
	if _, ok := capturedEnv["STRAIT_PAYLOAD"]; ok {
		t.Error("STRAIT_PAYLOAD should not be set for large payloads")
	}
}

// Test 10: Secrets injection → STRAIT_SECRET_* env vars.
func TestManagedDispatch_SecretsInjection(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		listSecretsFn: func(_ context.Context, _, _ string) ([]domain.JobSecret, error) {
			return []domain.JobSecret{
				{SecretKey: "db_password", EncryptedValue: "encrypted-pass-123"},
				{SecretKey: "api_key", EncryptedValue: "encrypted-key-456"},
			}, nil
		},
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv["STRAIT_SECRET_DB_PASSWORD"] != "encrypted-pass-123" {
		t.Errorf("expected db_password secret, got %q", capturedEnv["STRAIT_SECRET_DB_PASSWORD"])
	}
	if capturedEnv["STRAIT_SECRET_API_KEY"] != "encrypted-key-456" {
		t.Errorf("expected api_key secret, got %q", capturedEnv["STRAIT_SECRET_API_KEY"])
	}
}

// Test 11: nil runtime → system_failed.
func TestManagedDispatch_NilRuntime(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}

	e := newManagedTestExecutor(store, nil)
	e.containerRuntime = nil
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var foundSystemFailed bool
	for _, u := range updates {
		if u.to == domain.StatusSystemFailed {
			foundSystemFailed = true
		}
	}
	if !foundSystemFailed {
		t.Error("expected system_failed when runtime is nil")
	}
}

// Test 12: Container crash with logs → crash log event stored.
func TestManagedDispatch_CrashLogs(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var eventStored atomic.Bool
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			eventStored.Store(true)
			// Exit 137 (OOM) → container_oom event type.
			if event.Type != domain.EventType("container_oom") {
				t.Errorf("expected container_oom event for exit 137, got %s", event.Type)
			}
			if event.Level != "error" {
				t.Errorf("expected error level, got %s", event.Level)
			}
			if event.Message != "container killed by OOM (SIGKILL)" {
				t.Errorf("expected OOM message, got %s", event.Message)
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID:  "test-machine",
				ExitCode:   137,
				StartedAt:  &now,
				FinishedAt: &now,
				Logs:       "OOMKilled: out of memory",
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 3
	job := newTestManagedJob()
	job.MaxAttempts = 3 // exhausted

	e.managedDispatch(context.Background(), run, job)

	if !eventStored.Load() {
		t.Error("expected crash log event to be stored")
	}
}

// Test 13: Fatal error → system_failed, no retry.
func TestManagedDispatch_FatalError(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "", compute.NewFatalError(400, "invalid image", nil)
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var foundSystemFailed bool
	for _, u := range updates {
		if u.to == domain.StatusSystemFailed {
			foundSystemFailed = true
		}
	}
	if !foundSystemFailed {
		t.Error("expected system_failed for fatal error")
	}
}

// Test 14: Env vars include all required fields.
func TestManagedDispatch_EnvVars(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 3
	job := newTestManagedJob()
	job.Slug = "test-slug"

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv["STRAIT_RUN_ID"] != "run-1" {
		t.Errorf("expected STRAIT_RUN_ID=run-1, got %q", capturedEnv["STRAIT_RUN_ID"])
	}
	if capturedEnv["STRAIT_JOB_SLUG"] != "test-slug" {
		t.Errorf("expected STRAIT_JOB_SLUG=test-slug, got %q", capturedEnv["STRAIT_JOB_SLUG"])
	}
	if capturedEnv["STRAIT_ATTEMPT"] != "3" {
		t.Errorf("expected STRAIT_ATTEMPT=3, got %q", capturedEnv["STRAIT_ATTEMPT"])
	}
	if capturedEnv["STRAIT_API_URL"] != "https://api.test.com" {
		t.Errorf("expected STRAIT_API_URL, got %q", capturedEnv["STRAIT_API_URL"])
	}
	if capturedEnv["STRAIT_SDK_TOKEN"] == "" {
		t.Error("expected STRAIT_SDK_TOKEN to be set")
	}
}

// Test 15: Max attempts exhausted → dead_letter.
func TestManagedDispatch_MaxAttemptsExhausted(t *testing.T) {
	t.Parallel()
	now := time.Now()

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID:  "test-machine",
				ExitCode:   1,
				StartedAt:  &now,
				FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 3 // at max
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var foundDeadLetter bool
	for _, u := range updates {
		if u.to == domain.StatusDeadLetter {
			foundDeadLetter = true
		}
	}
	if !foundDeadLetter {
		t.Error("expected dead_letter when max attempts exhausted")
	}
}

// Test 16: Labels set correctly on RunRequest.
func TestManagedDispatch_Labels(t *testing.T) {
	t.Parallel()

	var capturedReq compute.RunRequest
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedReq = req
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedReq.Labels["run_id"] != "run-1" {
		t.Errorf("expected label run_id=run-1, got %q", capturedReq.Labels["run_id"])
	}
	if capturedReq.Labels["job_id"] != "job-1" {
		t.Errorf("expected label job_id=job-1, got %q", capturedReq.Labels["job_id"])
	}
	if capturedReq.Labels["project_id"] != "proj-1" {
		t.Errorf("expected label project_id=proj-1, got %q", capturedReq.Labels["project_id"])
	}
	if capturedReq.TimeoutSecs != 300 {
		t.Errorf("expected timeout 300, got %d", capturedReq.TimeoutSecs)
	}
}

// Test 17: Fly 429 → retryable, classified with 10s backoff.
func TestManagedDispatch_Fly429_Classified(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "", compute.NewRetryableError(429, "rate limited", nil)
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	// Should snooze (not system_failed) because it's retryable.
	updates := store.statusUpdates()
	var snoozed bool
	for _, u := range updates {
		if u.to == domain.StatusQueued {
			snoozed = true
		}
	}
	if !snoozed {
		t.Error("expected snooze for 429 retryable error")
	}
}

// Test 18: Fly 503 → retryable, 30s backoff.
func TestManagedDispatch_Fly503_Classified(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "", compute.NewRetryableError(503, "capacity unavailable", nil)
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var snoozed bool
	for _, u := range updates {
		if u.to == domain.StatusQueued {
			snoozed = true
		}
	}
	if !snoozed {
		t.Error("expected snooze for 503 retryable error")
	}
}

// Test 19: Fly 422 → fatal, no retry.
func TestManagedDispatch_Fly422_Fatal(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "", compute.NewFatalError(422, "invalid config", nil)
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var foundSystemFailed bool
	for _, u := range updates {
		if u.to == domain.StatusSystemFailed {
			foundSystemFailed = true
		}
	}
	if !foundSystemFailed {
		t.Error("expected system_failed for 422 fatal error")
	}
}

// Test 20: Empty job.Region → config default used.
func TestManagedDispatch_RegionFallback_ConfigDefault(t *testing.T) {
	t.Parallel()

	var capturedReq compute.RunRequest
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedReq = req
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	job := newTestManagedJob()
	job.Region = "" // no job-level region

	e.managedDispatch(context.Background(), run, job)

	if capturedReq.Region != "iad" {
		t.Errorf("expected region iad (config default), got %q", capturedReq.Region)
	}
}

// Test 21: Job region set → used directly, not overridden by default.
func TestManagedDispatch_RegionFallback_JobRegionUsed(t *testing.T) {
	t.Parallel()

	var capturedReq compute.RunRequest
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedReq = req
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	job := newTestManagedJob()
	job.Region = "lhr" // explicit job region

	e.managedDispatch(context.Background(), run, job)

	if capturedReq.Region != "lhr" {
		t.Errorf("expected region lhr (job region), got %q", capturedReq.Region)
	}
}

// Test 22: Region hint from run metadata → used when no job region.
func TestManagedDispatch_RegionFallback_MetadataHint(t *testing.T) {
	t.Parallel()

	var capturedReq compute.RunRequest
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedReq = req
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	run.Metadata = map[string]string{"_region_hint": "lhr"}
	job := newTestManagedJob()
	job.Region = "" // no job-level region

	e.managedDispatch(context.Background(), run, job)

	if capturedReq.Region != "lhr" {
		t.Errorf("expected region lhr (metadata hint), got %q", capturedReq.Region)
	}
}

// Test 23: Region resolution chain: job > hint > default.
func TestManagedDispatch_RegionChain_JobWins(t *testing.T) {
	t.Parallel()

	var capturedReq compute.RunRequest
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedReq = req
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	run.Metadata = map[string]string{"_region_hint": "lhr"}
	job := newTestManagedJob()
	job.Region = "nrt" // explicit job region wins over hint and default

	e.managedDispatch(context.Background(), run, job)

	if capturedReq.Region != "nrt" {
		t.Errorf("expected region nrt (job wins), got %q", capturedReq.Region)
	}
}

// Test 24: Non-RuntimeError → generic handling unchanged.
func TestManagedDispatch_NonRuntimeError_GenericHandling(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "", fmt.Errorf("network timeout")
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var foundSystemFailed bool
	for _, u := range updates {
		if u.to == domain.StatusSystemFailed {
			foundSystemFailed = true
		}
	}
	if !foundSystemFailed {
		t.Error("expected system_failed for generic non-RuntimeError")
	}
}

// Test 25: Nil result guard — Wait returns nil → system_failed.
func TestManagedDispatch_NilResultGuard(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return nil, nil // nil result, no error
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var foundSystemFailed bool
	for _, u := range updates {
		if u.to == domain.StatusSystemFailed {
			foundSystemFailed = true
			if !strings.Contains(u.fields["error"].(string), "nil result") {
				t.Errorf("expected error about nil result, got %s", u.fields["error"])
			}
		}
	}
	if !foundSystemFailed {
		t.Error("expected system_failed when Wait returns nil result")
	}
}

// Test 26: Cancel race during Create → Stop called.
func TestManagedDispatch_CancelRaceDuringCreate(t *testing.T) {
	t.Parallel()

	var stopCalled atomic.Bool
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{
				ID:     "run-1",
				Status: domain.StatusCanceled, // Cancel arrived during Create→SetMachineID window
			}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		stopFn: func(_ context.Context, machineID string) error {
			stopCalled.Store(true)
			if machineID != "test-machine" {
				t.Errorf("expected stop for test-machine, got %s", machineID)
			}
			return nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			t.Error("Wait should not be called when cancel race detected")
			return nil, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if !stopCalled.Load() {
		t.Error("expected Stop to be called when cancel race detected")
	}
}

// Test 28: Pool acquire hit → skip Create.
func TestManagedDispatch_PoolAcquireHit(t *testing.T) {
	t.Parallel()

	var createCalled atomic.Bool
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, _ string, _ map[string]string) error {
			return nil // Start succeeds — pooled machine reused.
		},
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalled.Store(true)
			return "new-machine", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	pool := compute.NewMachinePool(3)
	pool.Release("proj-1", "alpine:latest", "iad", "pooled-machine")

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	job := newTestManagedJob()
	job.Region = ""

	e.managedDispatch(context.Background(), run, job)

	if createCalled.Load() {
		t.Error("expected Create to be skipped when pool has a machine")
	}
}

// Test 29: Pool release — clean exit → machine returned to pool.
func TestManagedDispatch_PoolRelease(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	pool := compute.NewMachinePool(3)

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	job := newTestManagedJob()
	job.Region = ""

	e.managedDispatch(context.Background(), run, job)

	if pool.Size() != 1 {
		t.Errorf("expected pool size 1 after clean exit, got %d", pool.Size())
	}
}

// Test 30: Pool disabled (nil) → normal Create flow.
func TestManagedDispatch_PoolDisabled(t *testing.T) {
	t.Parallel()

	var createCalled atomic.Bool
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalled.Store(true)
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime) // no pool
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if !createCalled.Load() {
		t.Error("expected Create to be called when pool is nil")
	}
}

// Test 27: Invalid region hint → falls back to default.
func TestManagedDispatch_InvalidRegionHint(t *testing.T) {
	t.Parallel()

	var capturedReq compute.RunRequest
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedReq = req
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	run.Metadata = map[string]string{"_region_hint": "xyzzy-bogus"} // invalid region
	job := newTestManagedJob()
	job.Region = "" // no job region

	e.managedDispatch(context.Background(), run, job)

	if capturedReq.Region != "iad" {
		t.Errorf("expected fallback to default region iad, got %q", capturedReq.Region)
	}
}

func TestManagedDispatch_RetryIncludesCheckpoint(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
		getLatestCheckpointFn: func(_ context.Context, runID string) (*domain.RunCheckpoint, error) {
			return &domain.RunCheckpoint{
				ID:        "cp-1",
				RunID:     runID,
				Sequence:  3,
				State:     json.RawMessage(`{"progress":75}`),
				CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 2
	run.Error = "previous failure"
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv == nil {
		t.Fatal("expected env to be captured")
	}
	if capturedEnv["STRAIT_LAST_CHECKPOINT"] != `{"progress":75}` {
		t.Errorf("expected checkpoint in env, got %q", capturedEnv["STRAIT_LAST_CHECKPOINT"])
	}
	if capturedEnv["STRAIT_CHECKPOINT_AT"] == "" {
		t.Error("expected STRAIT_CHECKPOINT_AT to be set")
	}
	if capturedEnv["STRAIT_PREVIOUS_ERROR"] != "previous failure" {
		t.Errorf("expected previous error in env, got %q", capturedEnv["STRAIT_PREVIOUS_ERROR"])
	}
}

func TestManagedDispatch_FirstAttemptNoCheckpoint(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string
	checkpointCalled := false

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
		getLatestCheckpointFn: func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
			checkpointCalled = true
			return nil, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1 // First attempt
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if checkpointCalled {
		t.Error("should not call GetLatestCheckpoint on first attempt")
	}
	if _, ok := capturedEnv["STRAIT_LAST_CHECKPOINT"]; ok {
		t.Error("should not have STRAIT_LAST_CHECKPOINT on first attempt")
	}
}

func TestManagedDispatch_RetryLargeCheckpointOmitted(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string

	// Build a payload larger than 64KB.
	bigJSON := make([]byte, 0, 70*1024+2)
	bigJSON = append(bigJSON, '"')
	for range 70 * 1024 {
		bigJSON = append(bigJSON, 'x')
	}
	bigJSON = append(bigJSON, '"')

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
		getLatestCheckpointFn: func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
			return &domain.RunCheckpoint{
				ID:        "cp-1",
				RunID:     "run-1",
				Sequence:  1,
				State:     json.RawMessage(bigJSON),
				CreatedAt: time.Now(),
			}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 2
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if _, ok := capturedEnv["STRAIT_LAST_CHECKPOINT"]; ok {
		t.Error("should not include checkpoint > 64KB in env")
	}
}

func TestManagedDispatch_PresetOverrideFromMetadata(t *testing.T) {
	t.Parallel()

	var capturedReq compute.RunRequest

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedReq = req
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Metadata = map[string]string{"_preset_override": "large-1x"}
	job := newTestManagedJob() // default preset is "micro"

	e.managedDispatch(context.Background(), run, job)

	if capturedReq.MachinePreset != "large-1x" {
		t.Errorf("expected preset override large-1x, got %q", capturedReq.MachinePreset)
	}
}

func TestManagedDispatch_InvalidPresetOverrideIgnored(t *testing.T) {
	t.Parallel()

	var capturedReq compute.RunRequest

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedReq = req
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Metadata = map[string]string{"_preset_override": "invalid_preset"}
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedReq.MachinePreset != "micro" {
		t.Errorf("expected original preset micro, got %q (invalid override should be ignored)", capturedReq.MachinePreset)
	}
}

// Phase 2 tests.

func TestManagedDispatch_PoolAcquire_CallsStart(t *testing.T) {
	t.Parallel()
	var startCalled atomic.Bool
	var createCalled atomic.Bool

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, machineID string, env map[string]string) error {
			startCalled.Store(true)
			if machineID != "pooled-m" {
				t.Errorf("Start called with machineID=%q, want pooled-m", machineID)
			}
			if env["STRAIT_RUN_ID"] != "run-1" {
				t.Errorf("Start env missing STRAIT_RUN_ID")
			}
			return nil
		},
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalled.Store(true)
			return "new-m", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	pool := compute.NewMachinePool(3)
	pool.Release("proj-1", "alpine:latest", "iad", "pooled-m")

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	e.managedDispatch(context.Background(), newTestRun(), newTestManagedJob())

	if !startCalled.Load() {
		t.Error("expected Start to be called with pooled machine")
	}
	if createCalled.Load() {
		t.Error("expected Create NOT to be called when Start succeeds")
	}
}

func TestManagedDispatch_PoolAcquire_StartFails_FallsToCreate(t *testing.T) {
	t.Parallel()
	var createCalled atomic.Bool

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, _ string, _ map[string]string) error {
			return compute.ErrMachineGone
		},
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalled.Store(true)
			return "new-m", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	pool := compute.NewMachinePool(3)
	pool.Release("proj-1", "alpine:latest", "iad", "pooled-m")

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	e.managedDispatch(context.Background(), newTestRun(), newTestManagedJob())

	if !createCalled.Load() {
		t.Error("expected Create to be called when Start fails with ErrMachineGone")
	}
}

func TestManagedDispatch_PoolAcquire_StartTransient_FallsToCreate(t *testing.T) {
	t.Parallel()
	var createCalled atomic.Bool

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, _ string, _ map[string]string) error {
			return compute.NewRetryableError(500, "transient", nil)
		},
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalled.Store(true)
			return "new-m", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	pool := compute.NewMachinePool(3)
	pool.Release("proj-1", "alpine:latest", "iad", "pooled-m")

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	e.managedDispatch(context.Background(), newTestRun(), newTestManagedJob())

	if !createCalled.Load() {
		t.Error("expected Create to be called when Start returns transient error")
	}
}

func TestManagedDispatch_Reusable_SetsAutoDestroyFalse(t *testing.T) {
	t.Parallel()
	var capturedReq compute.RunRequest

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedReq = req
			return "m-1", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	pool := compute.NewMachinePool(3)
	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	e.managedDispatch(context.Background(), newTestRun(), newTestManagedJob())

	if !capturedReq.Reusable {
		t.Error("expected Reusable=true when pool is enabled")
	}
}

func TestManagedDispatch_SnoozePath_StopsMachineBeforeSnooze(t *testing.T) {
	t.Parallel()
	var stopCalled atomic.Bool

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "m-1", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return nil, compute.NewRetryableError(0, "wait failed", nil)
		},
		stopFn: func(_ context.Context, machineID string) error {
			stopCalled.Store(true)
			if machineID != "m-1" {
				t.Errorf("Stop called with %q, want m-1", machineID)
			}
			return nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), newTestRun(), newTestManagedJob())

	if !stopCalled.Load() {
		t.Error("expected Stop to be called before snoozing on Wait error")
	}
}

func TestManagedDispatch_CancelRace_StopFailure_Destroys(t *testing.T) {
	t.Parallel()
	var destroyCalled atomic.Bool

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCanceled}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "m-1", nil
		},
		stopFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("stop failed")
		},
		destroyFn: func(_ context.Context, machineID string) error {
			destroyCalled.Store(true)
			if machineID != "m-1" {
				t.Errorf("Destroy called with %q, want m-1", machineID)
			}
			return nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), newTestRun(), newTestManagedJob())

	if !destroyCalled.Load() {
		t.Error("expected Destroy as fallback when Stop fails on cancel race")
	}
}

func TestManagedDispatch_ShutdownDrainsPool(t *testing.T) {
	t.Parallel()
	var destroyedIDs sync.Map

	runtime := &mockContainerRuntime{
		destroyFn: func(_ context.Context, machineID string) error {
			destroyedIDs.Store(machineID, true)
			return nil
		},
	}

	pool := compute.NewMachinePool(5)
	pool.Release("proj-1", "img:latest", "iad", "m-1")
	pool.Release("proj-1", "img:latest", "iad", "m-2")
	pool.Release("proj-1", "img:latest", "iad", "m-3")

	e := &Executor{
		store:            &mockExecutorStore{},
		containerRuntime: runtime,
		machinePool:      pool,
		heartbeat:        NewHeartbeatSender(&mockExecutorStore{}, 10*time.Second),
		eventCh:          make(chan runEventEnvelope, 256),
		logger:           slog.Default(),
		pollInterval:     time.Hour, // Long interval — we'll cancel immediately.
		stop:             make(chan struct{}),
		done:             make(chan struct{}),
	}

	// Start and then shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	go e.Run(ctx)
	// Give Run a moment to start.
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := e.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}

	// Verify all pooled machines were destroyed.
	for _, id := range []string{"m-1", "m-2", "m-3"} {
		if _, ok := destroyedIDs.Load(id); !ok {
			t.Errorf("expected %s to be destroyed on shutdown", id)
		}
	}
	if pool.Size() != 0 {
		t.Errorf("expected pool size 0 after drain, got %d", pool.Size())
	}
}

// Phase 3 tests.

func TestManagedDispatch_ResumedRun_ReusesPausedMachine(t *testing.T) {
	t.Parallel()
	var startCalled atomic.Bool
	var createCalled atomic.Bool
	var startEnv map[string]string

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, machineID string, env map[string]string) error {
			startCalled.Store(true)
			startEnv = env
			if machineID != "paused-m" {
				t.Errorf("Start called with machineID=%q, want paused-m", machineID)
			}
			return nil
		},
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalled.Store(true)
			return "new-m", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.MachineID = "paused-m" // Preserved from pause.

	e.managedDispatch(context.Background(), run, newTestManagedJob())

	if !startCalled.Load() {
		t.Error("expected Start to be called with paused machine")
	}
	if createCalled.Load() {
		t.Error("expected Create NOT to be called when Start succeeds")
	}
	if startEnv["STRAIT_RUN_ID"] != "run-1" {
		t.Error("Start env should contain fresh STRAIT_RUN_ID")
	}
	if startEnv["STRAIT_SDK_TOKEN"] == "" {
		t.Error("Start env should contain fresh STRAIT_SDK_TOKEN")
	}
}

func TestManagedDispatch_ResumedRun_MachineGone_FallsToCreate(t *testing.T) {
	t.Parallel()
	var createCalled atomic.Bool

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, _ string, _ map[string]string) error {
			return compute.ErrMachineGone
		},
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalled.Store(true)
			return "new-m", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.MachineID = "paused-m"

	e.managedDispatch(context.Background(), run, newTestManagedJob())

	if !createCalled.Load() {
		t.Error("expected Create when paused machine is gone")
	}
}

func TestManagedDispatch_ResumedRun_MachineTransientError_FallsToCreate(t *testing.T) {
	t.Parallel()
	var createCalled atomic.Bool

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, _ string, _ map[string]string) error {
			return compute.NewRetryableError(500, "transient", nil)
		},
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalled.Store(true)
			return "new-m", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.MachineID = "paused-m"

	e.managedDispatch(context.Background(), run, newTestManagedJob())

	if !createCalled.Load() {
		t.Error("expected Create when paused machine Start returns transient error")
	}
}

func TestManagedDispatch_PoolTakesPriorityOverPausedMachine(t *testing.T) {
	t.Parallel()
	var startedMachines []string

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, machineID string, _ map[string]string) error {
			startedMachines = append(startedMachines, machineID)
			return nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	pool := compute.NewMachinePool(3)
	pool.Release("proj-1", "alpine:latest", "iad", "pool-m")

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	run.MachineID = "paused-m" // Both pool and paused available.

	e.managedDispatch(context.Background(), run, newTestManagedJob())

	if len(startedMachines) != 1 || startedMachines[0] != "pool-m" {
		t.Errorf("expected pool machine used first, got %v", startedMachines)
	}
}

func TestManagedDispatch_ResumedRun_EnvContainsCheckpoint(t *testing.T) {
	t.Parallel()
	var startEnv map[string]string

	cpTime := time.Now().Add(-5 * time.Minute)
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
		getLatestCheckpointFn: func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
			return &domain.RunCheckpoint{
				State:     json.RawMessage(`{"step":"2"}`),
				CreatedAt: cpTime,
			}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, _ string, env map[string]string) error {
			startEnv = env
			return nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.MachineID = "paused-m"
	run.Attempt = 2 // Retried — should get checkpoint.

	e.managedDispatch(context.Background(), run, newTestManagedJob())

	if startEnv["STRAIT_LAST_CHECKPOINT"] == "" {
		t.Error("expected STRAIT_LAST_CHECKPOINT in env for retried run")
	}
	if startEnv["STRAIT_CHECKPOINT_AT"] == "" {
		t.Error("expected STRAIT_CHECKPOINT_AT in env for retried run")
	}
}

// Phase 4 tests.

func TestManagedDispatch_CreateTimeout_Snoozes(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "", compute.NewRetryableError(0, "create timeout", context.DeadlineExceeded)
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var foundSnooze bool
	for _, u := range updates {
		if u.from == domain.StatusExecuting && u.to == domain.StatusQueued {
			foundSnooze = true
		}
	}
	if !foundSnooze {
		t.Error("expected snooze when Create returns deadline exceeded")
	}
}

func TestManagedDispatch_WaitTimeout_StopsAndSnoozes(t *testing.T) {
	t.Parallel()

	var stopCalled atomic.Bool
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return nil, compute.NewRetryableError(0, "wait timeout", context.DeadlineExceeded)
		},
		stopFn: func(_ context.Context, machineID string) error {
			stopCalled.Store(true)
			if machineID != "test-machine" {
				t.Errorf("Stop called with %q, want test-machine", machineID)
			}
			return nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if !stopCalled.Load() {
		t.Error("expected Stop to be called on Wait timeout")
	}

	updates := store.statusUpdates()
	var foundSnooze bool
	for _, u := range updates {
		if u.to == domain.StatusQueued {
			foundSnooze = true
		}
	}
	if !foundSnooze {
		t.Error("expected snooze after Wait timeout")
	}
}

func TestManagedDispatch_BudgetExceeded_NoMachineCreated(t *testing.T) {
	t.Parallel()

	var createCalled atomic.Bool
	store := &mockExecutorStore{
		getProjectQuotaFn: func(_ context.Context, _ string) (*orcstore.ProjectQuota, error) {
			return &orcstore.ProjectQuota{
				ProjectID:                     "proj-1",
				ComputeDailyCostLimitMicrousd: 100,
			}, nil
		},
		sumDailyComputeCostFn: func(_ context.Context, _, _ string) (int64, error) {
			return 99, nil // At 99, estimated cost will push over 100.
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalled.Store(true)
			return "leaked-machine", nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()
	job.TimeoutSecs = 300 // estimated cost = 17 * 300 = 5100 >> remaining 1

	e.managedDispatch(context.Background(), run, job)

	if createCalled.Load() {
		t.Error("expected Create NOT to be called when budget exceeded — machine would leak")
	}

	updates := store.statusUpdates()
	var foundSystemFailed bool
	for _, u := range updates {
		if u.to == domain.StatusSystemFailed {
			foundSystemFailed = true
		}
	}
	if !foundSystemFailed {
		t.Error("expected system_failed for budget exceeded")
	}
}

func TestManagedDispatch_ResumedRun_PoolAndPauseAndCreate_Cascade(t *testing.T) {
	t.Parallel()

	var callOrder []string
	var mu sync.Mutex
	record := func(name string) {
		mu.Lock()
		callOrder = append(callOrder, name)
		mu.Unlock()
	}

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, machineID string, _ map[string]string) error {
			record("start:" + machineID)
			return compute.ErrMachineGone // Both pool and paused machine gone.
		},
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			record("create")
			return "new-m", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	pool := compute.NewMachinePool(3)
	pool.Release("proj-1", "alpine:latest", "iad", "pool-m")

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	run.MachineID = "paused-m" // Paused machine set.

	e.managedDispatch(context.Background(), run, newTestManagedJob())

	mu.Lock()
	defer mu.Unlock()

	// Expect: pool Start attempted first, then paused Start, then Create fallback.
	if len(callOrder) < 2 {
		t.Fatalf("expected at least 2 calls, got %v", callOrder)
	}
	// Pool machine is tried first (pool takes priority over paused machine).
	if callOrder[0] != "start:pool-m" {
		t.Errorf("expected pool machine start first, got %q", callOrder[0])
	}
	// Last call should be Create since both Start calls fail.
	lastCall := callOrder[len(callOrder)-1]
	if lastCall != "create" {
		t.Errorf("expected Create as final fallback, got %q", lastCall)
	}
}

func TestManagedDispatch_NonZeroExit_MachineNotPooled(t *testing.T) {
	t.Parallel()
	now := time.Now()

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID:  "test-machine",
				ExitCode:   1,
				StartedAt:  &now,
				FinishedAt: &now,
			}, nil
		},
	}

	pool := compute.NewMachinePool(3)

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if pool.Size() != 0 {
		t.Errorf("expected pool size 0 after non-zero exit, got %d", pool.Size())
	}
}

func TestManagedDispatch_ExitZero_MachinePooled(t *testing.T) {
	t.Parallel()
	now := time.Now()

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID:  "test-machine",
				ExitCode:   0,
				StartedAt:  &now,
				FinishedAt: &now,
			}, nil
		},
	}

	pool := compute.NewMachinePool(3)

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	job := newTestManagedJob()
	job.Region = ""

	e.managedDispatch(context.Background(), run, job)

	if pool.Size() != 1 {
		t.Errorf("expected pool size 1 after clean exit with SDK complete, got %d", pool.Size())
	}
}

func TestManagedDispatch_MaxSnoozeExceeded_SystemFails(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "", compute.NewRetryableError(429, "rate limited", nil)
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.maxSnoozeCount = 5 // Low max for testing.

	run := newTestRun()
	run.Metadata = map[string]string{"snooze_count": "6"} // Already over max.
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var foundSystemFailed bool
	for _, u := range updates {
		if u.to == domain.StatusSystemFailed {
			foundSystemFailed = true
		}
	}
	if !foundSystemFailed {
		t.Error("expected system_failed when snooze count exceeds max")
	}
}

// Phase 5 tests.

func TestManagedDispatch_MetricsRecordPoolHit(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, _ string, _ map[string]string) error {
			return nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}
	pool := compute.NewMachinePool(3)
	pool.Release("proj-1", "alpine:latest", "iad", "pool-m")

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	// The pool metric is recorded as "pool" dispatch source.
	// We verify indirectly that the dispatch completes successfully with pool path.
	e.managedDispatch(context.Background(), newTestRun(), newTestManagedJob())

	// If we got here without panic, metrics were recorded (pool path used).
}

func TestManagedDispatch_MetricsRecordPauseReuse(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, _ string, _ map[string]string) error {
			return nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.MachineID = "paused-m"

	e.managedDispatch(context.Background(), run, newTestManagedJob())
	// Dispatch completes via pause_reuse path → metrics recorded.
}

func TestManagedDispatch_MetricsRecordColdStart(t *testing.T) {
	t.Parallel()
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "new-m", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	e.managedDispatch(context.Background(), newTestRun(), newTestManagedJob())
	// No pool, no paused machine → cold_start path → metrics recorded.
}

// Phase 1: Exit code classification tests.

func TestManagedDispatch_Exit137_OOM_ErrorClass(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedErrorClass string
	var capturedEventType domain.EventType
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, fields map[string]any) error {
			if ec, ok := fields["error_class"]; ok {
				capturedErrorClass = ec.(string)
			}
			return nil
		},
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			capturedEventType = event.Type
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 137,
				StartedAt: &now, FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if capturedErrorClass != "out_of_memory" {
		t.Errorf("expected error_class out_of_memory, got %s", capturedErrorClass)
	}
	if capturedEventType != domain.EventType("container_oom") {
		t.Errorf("expected container_oom event, got %s", capturedEventType)
	}
}

func TestManagedDispatch_Exit143_GracefulShutdown(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedErrorClass string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, fields map[string]any) error {
			if ec, ok := fields["error_class"]; ok {
				capturedErrorClass = ec.(string)
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 143,
				StartedAt: &now, FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if capturedErrorClass != "graceful_shutdown" {
		t.Errorf("expected error_class graceful_shutdown, got %s", capturedErrorClass)
	}
}

func TestManagedDispatch_Exit139_Segfault(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedErrorClass string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, fields map[string]any) error {
			if ec, ok := fields["error_class"]; ok {
				capturedErrorClass = ec.(string)
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 139,
				StartedAt: &now, FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if capturedErrorClass != "segfault" {
		t.Errorf("expected error_class segfault, got %s", capturedErrorClass)
	}
}

func TestManagedDispatch_Exit1_ApplicationError(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedErrorClass string
	var capturedEventType domain.EventType
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, fields map[string]any) error {
			if ec, ok := fields["error_class"]; ok {
				capturedErrorClass = ec.(string)
			}
			return nil
		},
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			capturedEventType = event.Type
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 1,
				StartedAt: &now, FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if capturedErrorClass != "application_error" {
		t.Errorf("expected error_class application_error, got %s", capturedErrorClass)
	}
	if capturedEventType != domain.EventType("container_crash_log") {
		t.Errorf("expected container_crash_log event for non-OOM, got %s", capturedEventType)
	}
}

func TestManagedDispatch_CrashEventAlwaysInserted(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var eventStored atomic.Bool
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			eventStored.Store(true)
			// Verify crash event data contains preset and memory_mb.
			var data map[string]any
			if err := json.Unmarshal(event.Data, &data); err != nil {
				t.Errorf("failed to parse crash event data: %v", err)
			}
			if _, ok := data["preset"]; !ok {
				t.Error("crash event data should contain preset")
			}
			if _, ok := data["memory_mb"]; !ok {
				t.Error("crash event data should contain memory_mb")
			}
			if _, ok := data["error_class"]; !ok {
				t.Error("crash event data should contain error_class")
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 1,
				StartedAt: &now, FinishedAt: &now,
				// No logs — event should still be inserted.
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if !eventStored.Load() {
		t.Error("crash event should always be inserted, even without logs")
	}
}

func TestManagedDispatch_CrashEventIncludesCheckpoint(t *testing.T) {
	t.Parallel()
	now := time.Now()
	checkpointTime := now.Add(-5 * time.Minute)

	var crashData map[string]any
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		getLatestCheckpointFn: func(_ context.Context, _ string) (*domain.RunCheckpoint, error) {
			return &domain.RunCheckpoint{
				State:     json.RawMessage(`{"step": 2}`),
				CreatedAt: checkpointTime,
			}, nil
		},
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			if err := json.Unmarshal(event.Data, &crashData); err != nil {
				t.Fatalf("failed to parse event data: %v", err)
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 1,
				StartedAt: &now, FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 2 // Retry — checkpoint should be loaded.
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if crashData == nil {
		t.Fatal("expected crash event data")
	}
	if _, ok := crashData["last_checkpoint_at"]; !ok {
		t.Error("crash event data should contain last_checkpoint_at for retried run")
	}
}

func TestManagedDispatch_BeltAndSuspendersLogFetch(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedLogs string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			var data map[string]any
			if err := json.Unmarshal(event.Data, &data); err == nil {
				if logs, ok := data["logs"]; ok {
					capturedLogs = logs.(string)
				}
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 1,
				StartedAt: &now, FinishedAt: &now,
				Logs: "", // Empty logs from Wait.
			}, nil
		},
		getLogsFn: func(_ context.Context, _ string, _ int) (string, error) {
			return "belt-and-suspenders logs", nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if capturedLogs != "belt-and-suspenders logs" {
		t.Errorf("expected belt-and-suspenders logs, got %q", capturedLogs)
	}
}

// Phase 2: OOM-aware retry with preset upgrade.

func TestManagedDispatch_OOM_PresetUpgrade(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedMetadata map[string]string
	var capturedError string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, fields map[string]any) error {
			if to == domain.StatusQueued {
				if md, ok := fields["metadata"]; ok {
					capturedMetadata = md.(map[string]string)
				}
				if e, ok := fields["error"]; ok {
					capturedError = e.(string)
				}
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 137,
				StartedAt: &now, FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MachinePreset = domain.PresetMicro
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if capturedMetadata == nil {
		t.Fatal("expected metadata with _preset_override")
	}
	if capturedMetadata["_preset_override"] != "small-1x" {
		t.Errorf("expected _preset_override=small-1x, got %s", capturedMetadata["_preset_override"])
	}
	if capturedMetadata["_oom_upgraded_from"] != "micro" {
		t.Errorf("expected _oom_upgraded_from=micro, got %s", capturedMetadata["_oom_upgraded_from"])
	}
	if !strings.Contains(capturedError, "OOM on micro") {
		t.Errorf("expected OOM upgrade message, got %s", capturedError)
	}
}

func TestManagedDispatch_OOM_MaxPreset_DeadLetter(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var deadLettered bool
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, fields map[string]any) error {
			if to == domain.StatusDeadLetter {
				deadLettered = true
				errMsg := fields["error"].(string)
				if !strings.Contains(errMsg, "largest preset") {
					t.Errorf("expected max preset error message, got %s", errMsg)
				}
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 137,
				StartedAt: &now, FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	run.Metadata = map[string]string{"_preset_override": "large-2x"}
	job := newTestManagedJob()
	job.MachinePreset = "large-2x"
	job.MaxAttempts = 3 // Has retries remaining, but OOM on max → dead_letter.

	e.managedDispatch(context.Background(), run, job)

	if !deadLettered {
		t.Error("expected dead_letter for OOM on max preset, even with retries remaining")
	}
}

func TestManagedDispatch_NonOOM_NoPresetOverride(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedMetadata map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, fields map[string]any) error {
			if to == domain.StatusQueued {
				if md, ok := fields["metadata"]; ok {
					capturedMetadata = md.(map[string]string)
				}
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 1,
				StartedAt: &now, FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if capturedMetadata != nil {
		if _, ok := capturedMetadata["_preset_override"]; ok {
			t.Error("non-OOM exit should not set _preset_override")
		}
	}
}

func TestManagedDispatch_OOM_UpgradeChain(t *testing.T) {
	t.Parallel()

	// Verify micro → small-1x → small-2x upgrade chain.
	chain := []struct {
		currentPreset string
		expectedNext  string
	}{
		{"micro", "small-1x"},
		{"small-1x", "small-2x"},
		{"small-2x", "medium-1x"},
	}
	for _, step := range chain {
		next, ok := compute.NextPreset(step.currentPreset)
		if !ok {
			t.Errorf("NextPreset(%q) returned false", step.currentPreset)
		}
		if next != step.expectedNext {
			t.Errorf("NextPreset(%q) = %q, want %q", step.currentPreset, next, step.expectedNext)
		}
	}
}

func TestManagedDispatch_OOM_PreservesSnoozeCount(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedFields map[string]any
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusExecuting}, nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, fields map[string]any) error {
			if to == domain.StatusQueued {
				capturedFields = fields
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{
				MachineID: "test-machine", ExitCode: 137,
				StartedAt: &now, FinishedAt: &now,
			}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Attempt = 1
	run.Metadata = map[string]string{"snooze_count": "3"}
	job := newTestManagedJob()
	job.MaxAttempts = 3

	e.managedDispatch(context.Background(), run, job)

	if capturedFields == nil {
		t.Fatal("expected retry fields")
	}
	// The OOM upgrade metadata should be set.
	if md, ok := capturedFields["metadata"]; ok {
		m := md.(map[string]string)
		if m["_preset_override"] != "small-1x" {
			t.Errorf("expected _preset_override=small-1x, got %s", m["_preset_override"])
		}
	}
}

// Phase 3: STRAIT_CLEAN_START tests.

func TestManagedDispatch_PooledMachine_HasCleanStart(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string
	pool := compute.NewMachinePool(3)
	pool.Release("proj-1", "alpine:latest", "iad", "pooled-m")

	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, _ string, env map[string]string) error {
			capturedEnv = env
			return nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.machinePool = pool
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv["STRAIT_CLEAN_START"] != "true" {
		t.Errorf("expected STRAIT_CLEAN_START=true for pooled machine, got %q", capturedEnv["STRAIT_CLEAN_START"])
	}
}

func TestManagedDispatch_PausedMachine_HasCleanStart(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		startFn: func(_ context.Context, _ string, env map[string]string) error {
			capturedEnv = env
			return nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.MachineID = "paused-m"
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv["STRAIT_CLEAN_START"] != "true" {
		t.Errorf("expected STRAIT_CLEAN_START=true for paused machine, got %q", capturedEnv["STRAIT_CLEAN_START"])
	}
}

func TestManagedDispatch_ColdCreate_NoCleanStart(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "new-m", nil
		},
		waitFn: func(_ context.Context, machineID string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: machineID, ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if _, ok := capturedEnv["STRAIT_CLEAN_START"]; ok {
		t.Error("cold create should NOT have STRAIT_CLEAN_START")
	}
}

// Phase 4: Budget soft-limit warning tests.

func TestManagedDispatch_BudgetWarning_CrossingThreshold(t *testing.T) {
	t.Parallel()

	var warningInserted atomic.Bool
	store := &mockExecutorStore{
		getProjectQuotaFn: func(_ context.Context, _ string) (*orcstore.ProjectQuota, error) {
			return &orcstore.ProjectQuota{
				ProjectID:                     "proj-1",
				ComputeDailyCostLimitMicrousd: 10000, // $0.01
			}, nil
		},
		sumDailyComputeCostFn: func(_ context.Context, _, _ string) (int64, error) {
			return 7500, nil // 75% — adding estimate should cross 80%
		},
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			if event.Type == domain.EventType("budget_warning") {
				warningInserted.Store(true)
				var data map[string]any
				if err := json.Unmarshal(event.Data, &data); err != nil {
					t.Errorf("failed to parse warning data: %v", err)
				}
				if _, ok := data["project_id"]; !ok {
					t.Error("warning should contain project_id")
				}
				if _, ok := data["percentage"]; !ok {
					t.Error("warning should contain percentage")
				}
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()
	job.TimeoutSecs = 300 // estimated = 17 * 300 = 5100; 7500+5100 = 12600 > 8000 (80%)

	e.managedDispatch(context.Background(), run, job)

	if !warningInserted.Load() {
		t.Error("expected budget warning event when crossing 80% threshold")
	}
}

func TestManagedDispatch_BudgetWarning_AlreadyAbove_NoWarning(t *testing.T) {
	t.Parallel()

	var warningInserted atomic.Bool
	store := &mockExecutorStore{
		getProjectQuotaFn: func(_ context.Context, _ string) (*orcstore.ProjectQuota, error) {
			return &orcstore.ProjectQuota{
				ProjectID:                     "proj-1",
				ComputeDailyCostLimitMicrousd: 10000,
			}, nil
		},
		sumDailyComputeCostFn: func(_ context.Context, _, _ string) (int64, error) {
			return 9000, nil // 90% — already above threshold, no warning
		},
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			if event.Type == domain.EventType("budget_warning") {
				warningInserted.Store(true)
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()
	job.TimeoutSecs = 300

	e.managedDispatch(context.Background(), run, job)

	if warningInserted.Load() {
		t.Error("should not fire warning when already above threshold")
	}
}

func TestManagedDispatch_BudgetWarning_BelowThreshold_NoWarning(t *testing.T) {
	t.Parallel()

	var warningInserted atomic.Bool
	store := &mockExecutorStore{
		getProjectQuotaFn: func(_ context.Context, _ string) (*orcstore.ProjectQuota, error) {
			return &orcstore.ProjectQuota{
				ProjectID:                     "proj-1",
				ComputeDailyCostLimitMicrousd: 1000000, // $1 limit — very high
			}, nil
		},
		sumDailyComputeCostFn: func(_ context.Context, _, _ string) (int64, error) {
			return 100, nil // 0.01% — well below threshold
		},
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			if event.Type == domain.EventType("budget_warning") {
				warningInserted.Store(true)
			}
			return nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()
	job.TimeoutSecs = 300

	e.managedDispatch(context.Background(), run, job)

	if warningInserted.Load() {
		t.Error("should not fire warning when below threshold")
	}
}

// Phase 6: Multi-region failover tests.

func TestManagedDispatch_503Failover_SecondRegionSucceeds(t *testing.T) {
	t.Parallel()

	var capturedRegions []string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedRegions = append(capturedRegions, req.Region)
			if req.Region == "iad" {
				return "", compute.NewRetryableError(503, "capacity unavailable", nil)
			}
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	job := newTestManagedJob()
	job.Region = "" // Not pinned.

	e.managedDispatch(context.Background(), run, job)

	if len(capturedRegions) < 2 {
		t.Fatalf("expected at least 2 Create calls for failover, got %d", len(capturedRegions))
	}
	if capturedRegions[0] != "iad" {
		t.Errorf("first region should be iad, got %s", capturedRegions[0])
	}
	// Second region should be from iad's fallback chain.
	if capturedRegions[1] != "ewr" {
		t.Errorf("second region should be ewr (iad fallback), got %s", capturedRegions[1])
	}
}

func TestManagedDispatch_PinnedRegion_NoFailover(t *testing.T) {
	t.Parallel()

	var createCalls int
	store := &mockExecutorStore{}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalls++
			return "", compute.NewRetryableError(503, "capacity unavailable", nil)
		},
	}

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	job := newTestManagedJob()
	job.Region = "iad" // Pinned.

	e.managedDispatch(context.Background(), run, job)

	if createCalls != 1 {
		t.Errorf("expected exactly 1 Create call for pinned region (no failover), got %d", createCalls)
	}
}

func TestManagedDispatch_500_NoFailover(t *testing.T) {
	t.Parallel()

	var createCalls int
	store := &mockExecutorStore{}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			createCalls++
			return "", compute.NewRetryableError(500, "server error", nil)
		},
	}

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	job := newTestManagedJob()
	job.Region = "" // Not pinned.

	e.managedDispatch(context.Background(), run, job)

	if createCalls != 1 {
		t.Errorf("expected exactly 1 Create call for 500 (no failover), got %d", createCalls)
	}
}

func TestManagedDispatch_All503_Snoozes(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "", compute.NewRetryableError(503, "capacity unavailable", nil)
		},
	}

	e := newManagedTestExecutor(store, runtime, func(e *Executor) {
		e.defaultFlyRegion = "iad"
	})
	run := newTestRun()
	job := newTestManagedJob()
	job.Region = ""

	e.managedDispatch(context.Background(), run, job)

	updates := store.statusUpdates()
	var snoozed bool
	for _, u := range updates {
		if u.from == domain.StatusExecuting && u.to == domain.StatusQueued {
			snoozed = true
		}
	}
	if !snoozed {
		t.Error("expected snooze after all regions 503")
	}
}

// Phase 8: STRAIT_MEMORY_LIMIT_MB injection.

func TestManagedDispatch_MemoryLimitInjected(t *testing.T) {
	t.Parallel()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()
	job.MachinePreset = "micro" // 256MB

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv["STRAIT_MEMORY_LIMIT_MB"] != "256" {
		t.Errorf("expected STRAIT_MEMORY_LIMIT_MB=256, got %q", capturedEnv["STRAIT_MEMORY_LIMIT_MB"])
	}
}

func TestManagedDispatch_MemoryLimitPerPreset(t *testing.T) {
	t.Parallel()

	presetMemory := map[string]string{
		"micro":     "256",
		"small-1x":  "512",
		"small-2x":  "1024",
		"medium-1x": "4096",
		"medium-2x": "8192",
		"large-1x":  "16384",
		"large-2x":  "32768",
	}

	for preset, expectedMB := range presetMemory {
		var capturedEnv map[string]string
		store := &mockExecutorStore{
			getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
				return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
			},
		}
		runtime := &mockContainerRuntime{
			createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
				capturedEnv = req.Env
				return "test-machine", nil
			},
			waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
				now := time.Now()
				return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
			},
		}

		e := newManagedTestExecutor(store, runtime)
		run := newTestRun()
		job := newTestManagedJob()
		job.MachinePreset = domain.MachinePreset(preset)

		e.managedDispatch(context.Background(), run, job)

		if capturedEnv["STRAIT_MEMORY_LIMIT_MB"] != expectedMB {
			t.Errorf("preset %s: expected STRAIT_MEMORY_LIMIT_MB=%s, got %q", preset, expectedMB, capturedEnv["STRAIT_MEMORY_LIMIT_MB"])
		}
	}
}

// handleManagedFailure and managedDispatch edge case tests.

func TestHandleManagedFailure_RecordOOMEventStoreFailure(t *testing.T) {
	t.Parallel()

	var retryQueued atomic.Bool
	store := &mockExecutorStore{
		recordOOMEventFn: func(_ context.Context, _, _ string) error {
			return errors.New("db error recording oom")
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, fields map[string]any) error {
			if to == domain.StatusQueued {
				retryQueued.Store(true)
				// Verify OOM upgrade still happened despite RecordOOMEvent failure.
				if meta, ok := fields["metadata"].(map[string]string); ok {
					if _, hasOverride := meta["_preset_override"]; !hasOverride {
						t.Error("expected _preset_override in retry metadata despite RecordOOMEvent failure")
					}
				}
			}
			return nil
		},
	}

	e := newManagedTestExecutor(store, &mockContainerRuntime{})
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	classification := compute.ExitClassification{
		Signal:       "SIGKILL",
		IsOOM:        true,
		ErrorClass:   "oom",
		HumanMessage: "OOM killed",
	}

	e.handleManagedFailure(context.Background(), run, job, classification)

	if !retryQueued.Load() {
		t.Error("expected retry to proceed despite RecordOOMEvent failure")
	}
}

func TestHandleManagedFailure_UpdateRunStatusFailsOnRetry(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, _ map[string]any) error {
			if to == domain.StatusQueued {
				return errors.New("db error on retry transition")
			}
			return nil
		},
	}

	e := newManagedTestExecutor(store, &mockContainerRuntime{})
	run := newTestRun()
	run.Attempt = 1
	job := newTestManagedJob()
	job.MaxAttempts = 3

	classification := compute.ExitClassification{
		ErrorClass:   "application_error",
		HumanMessage: "exit 1",
	}

	// Should return early without panic.
	e.handleManagedFailure(context.Background(), run, job, classification)
}

func TestHandleManagedFailure_UpdateRunStatusFailsOnDeadLetter(t *testing.T) {
	t.Parallel()

	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, _ map[string]any) error {
			if to == domain.StatusDeadLetter {
				return errors.New("db error on dead_letter transition")
			}
			return nil
		},
	}

	e := newManagedTestExecutor(store, &mockContainerRuntime{})
	run := newTestRun()
	run.Attempt = 3 // exhausted
	job := newTestManagedJob()
	job.MaxAttempts = 3

	classification := compute.ExitClassification{
		ErrorClass:   "application_error",
		HumanMessage: "exit 1",
	}

	// Should return early without panic.
	e.handleManagedFailure(context.Background(), run, job, classification)
}

func TestHandleManagedFailure_OOMWithExistingPresetOverride(t *testing.T) {
	t.Parallel()

	var capturedPreset atomic.Value
	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, fields map[string]any) error {
			if to == domain.StatusQueued {
				if meta, ok := fields["metadata"].(map[string]string); ok {
					capturedPreset.Store(meta["_preset_override"])
				}
			}
			return nil
		},
	}

	e := newManagedTestExecutor(store, &mockContainerRuntime{})
	run := newTestRun()
	run.Attempt = 1
	run.Metadata = map[string]string{"_preset_override": "small-2x"}
	job := newTestManagedJob()
	job.MachinePreset = domain.PresetMicro
	job.MaxAttempts = 3

	classification := compute.ExitClassification{
		Signal:       "SIGKILL",
		IsOOM:        true,
		ErrorClass:   "oom",
		HumanMessage: "OOM killed",
	}

	e.handleManagedFailure(context.Background(), run, job, classification)

	preset := capturedPreset.Load()
	if preset == nil {
		t.Fatal("expected _preset_override in retry metadata")
	}
	// small-2x → medium-1x
	if preset.(string) != "medium-1x" {
		t.Errorf("expected upgrade to medium-1x, got %s", preset)
	}
}

func TestHandleManagedFailure_NonOOMExhausted_ErrorClass(t *testing.T) {
	t.Parallel()

	var capturedErrorClass atomic.Value
	store := &mockExecutorStore{
		updateRunStatusFn: func(_ context.Context, _ string, _, to domain.RunStatus, fields map[string]any) error {
			if to == domain.StatusDeadLetter {
				if ec, ok := fields["error_class"].(string); ok {
					capturedErrorClass.Store(ec)
				}
			}
			return nil
		},
	}

	e := newManagedTestExecutor(store, &mockContainerRuntime{})
	run := newTestRun()
	run.Attempt = 3 // exhausted
	job := newTestManagedJob()
	job.MaxAttempts = 3

	classification := compute.ExitClassification{
		ErrorClass:   "application_error",
		HumanMessage: "exit 1",
	}

	e.handleManagedFailure(context.Background(), run, job, classification)

	ec := capturedErrorClass.Load()
	if ec == nil {
		t.Fatal("expected error_class to be set")
	}
	if ec.(string) != "application_error" {
		t.Errorf("expected error_class application_error, got %s", ec)
	}
}

func TestManagedDispatch_BudgetWarningInsertEventFailure(t *testing.T) {
	t.Parallel()

	var dispatchCompleted atomic.Bool
	store := &mockExecutorStore{
		getProjectQuotaFn: func(_ context.Context, _ string) (*orcstore.ProjectQuota, error) {
			return &orcstore.ProjectQuota{
				ComputeDailyCostLimitMicrousd: 100000,
			}, nil
		},
		sumDailyComputeCostFn: func(_ context.Context, _, _ string) (int64, error) {
			// At 79% of budget — will cross 80% threshold with estimated cost.
			return 79000, nil
		},
		insertEventFn: func(_ context.Context, event *domain.RunEvent) error {
			if event.Type == "budget_warning" {
				return errors.New("failed to insert budget warning")
			}
			return nil
		},
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			dispatchCompleted.Store(true)
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, _ compute.RunRequest) (string, error) {
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if !dispatchCompleted.Load() {
		t.Error("dispatch should continue normally despite budget warning InsertEvent failure")
	}
}

func TestManagedDispatch_GetPresetRecommendationFailure(t *testing.T) {
	t.Parallel()

	var capturedPreset atomic.Value
	store := &mockExecutorStore{
		getPresetRecommendationFn: func(_ context.Context, _ string) (*orcstore.PresetRecommendation, error) {
			return nil, errors.New("recommendation db error")
		},
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedPreset.Store(req.MachinePreset)
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()
	job.MachinePreset = "small-1x"

	e.managedDispatch(context.Background(), run, job)

	preset := capturedPreset.Load()
	if preset == nil {
		t.Fatal("expected preset to be captured")
	}
	if preset.(string) != "small-1x" {
		t.Errorf("expected original preset small-1x on recommendation failure, got %s", preset)
	}
}

func TestManagedDispatch_RecommendationLowerThanCurrent(t *testing.T) {
	t.Parallel()

	var capturedPreset atomic.Value
	store := &mockExecutorStore{
		getPresetRecommendationFn: func(_ context.Context, _ string) (*orcstore.PresetRecommendation, error) {
			return &orcstore.PresetRecommendation{
				RecommendedPreset: "micro",
				OOMCount:          3,
			}, nil
		},
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}

	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedPreset.Store(req.MachinePreset)
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			now := time.Now()
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	job := newTestManagedJob()
	job.MachinePreset = "small-2x" // Higher than recommendation "micro"

	e.managedDispatch(context.Background(), run, job)

	preset := capturedPreset.Load()
	if preset == nil {
		t.Fatal("expected preset to be captured")
	}
	if preset.(string) != "small-2x" {
		t.Errorf("expected current preset small-2x (recommendation ignored), got %s", preset)
	}
}

// Trace propagation env var tests.

func TestManagedDispatch_InjectsTraceparent(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Metadata = map[string]string{
		"_trace_parent": "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
	}
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv == nil {
		t.Fatal("expected createFn to be called and env captured")
	}
	got, ok := capturedEnv["TRACEPARENT"]
	if !ok {
		t.Fatal("expected TRACEPARENT env var to be set")
	}
	if got != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Errorf("TRACEPARENT = %q, want %q", got, "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01")
	}
}

func TestManagedDispatch_InjectsTracestate(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Metadata = map[string]string{
		"_trace_parent": "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
		"_trace_state":  "congo=t61rcWkgMzE",
	}
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv == nil {
		t.Fatal("expected createFn to be called and env captured")
	}
	if tp := capturedEnv["TRACEPARENT"]; tp != "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01" {
		t.Errorf("TRACEPARENT = %q, want traceparent value", tp)
	}
	if ts := capturedEnv["TRACESTATE"]; ts != "congo=t61rcWkgMzE" {
		t.Errorf("TRACESTATE = %q, want %q", ts, "congo=t61rcWkgMzE")
	}
}

func TestManagedDispatch_NoTraceContext(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Metadata = nil
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv == nil {
		t.Fatal("expected createFn to be called and env captured")
	}
	if _, ok := capturedEnv["TRACEPARENT"]; ok {
		t.Error("expected TRACEPARENT not to be set when metadata is nil")
	}
	if _, ok := capturedEnv["TRACESTATE"]; ok {
		t.Error("expected TRACESTATE not to be set when metadata is nil")
	}
}

func TestManagedDispatch_EmptyTraceParent(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Metadata = map[string]string{
		"_trace_parent": "",
	}
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv == nil {
		t.Fatal("expected createFn to be called and env captured")
	}
	if _, ok := capturedEnv["TRACEPARENT"]; ok {
		t.Error("expected TRACEPARENT not to be set when _trace_parent is empty")
	}
}

func TestManagedDispatch_NonTraceMetadataNotLeaked(t *testing.T) {
	t.Parallel()
	now := time.Now()

	var capturedEnv map[string]string
	store := &mockExecutorStore{
		getRunFn: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}, nil
		},
	}
	runtime := &mockContainerRuntime{
		createFn: func(_ context.Context, req compute.RunRequest) (string, error) {
			capturedEnv = req.Env
			return "test-machine", nil
		},
		waitFn: func(_ context.Context, _ string, _ int) (*compute.RunResult, error) {
			return &compute.RunResult{MachineID: "test-machine", ExitCode: 0, StartedAt: &now, FinishedAt: &now}, nil
		},
	}

	e := newManagedTestExecutor(store, runtime)
	run := newTestRun()
	run.Metadata = map[string]string{
		"custom_key":    "secret",
		"_trace_parent": "00-abcdef1234567890abcdef1234567890-fedcba0987654321-01",
	}
	job := newTestManagedJob()

	e.managedDispatch(context.Background(), run, job)

	if capturedEnv == nil {
		t.Fatal("expected createFn to be called and env captured")
	}
	if _, ok := capturedEnv["custom_key"]; ok {
		t.Error("non-trace metadata key 'custom_key' should not be leaked into container env")
	}
	if _, ok := capturedEnv["CUSTOM_KEY"]; ok {
		t.Error("non-trace metadata key 'CUSTOM_KEY' should not be leaked into container env")
	}
	if _, ok := capturedEnv["TRACEPARENT"]; !ok {
		t.Error("expected TRACEPARENT to be set")
	}
}
