package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
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
	metrics, _, _, _ := telemetry.InitMetrics("test-managed")
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
			if event.Type != domain.EventType("container_crash_log") {
				t.Errorf("expected container_crash_log event, got %s", event.Type)
			}
			if event.Level != "error" {
				t.Errorf("expected error level, got %s", event.Level)
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
