package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"orchestrator/internal/domain"
	"orchestrator/internal/pubsub"
	"orchestrator/internal/testutil"
)

// Mock publisher for worker tests.

type mockWorkerPublisher struct {
	publishFn func(ctx context.Context, channel string, data []byte) error
	calls     atomic.Int32
}

func (m *mockWorkerPublisher) Publish(ctx context.Context, channel string, data []byte) error {
	m.calls.Add(1)
	if m.publishFn != nil {
		return m.publishFn(ctx, channel, data)
	}
	return nil
}

func (m *mockWorkerPublisher) Subscribe(_ context.Context, _ string) (*pubsub.Subscription, error) {
	return nil, errors.New("not implemented")
}

func (m *mockWorkerPublisher) Close() error { return nil }

var _ pubsub.Publisher = (*mockWorkerPublisher)(nil)

// Mock workflow callback.

type mockWorkflowCallback struct {
	onTerminalFn func(ctx context.Context, run *domain.JobRun) error
	calls        atomic.Int32
}

func (m *mockWorkflowCallback) OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error {
	m.calls.Add(1)
	if m.onTerminalFn != nil {
		return m.onTerminalFn(ctx, run)
	}
	return nil
}

// publishEvent tests.

func TestPublishEvent_NilPublisher(t *testing.T) {
	t.Parallel()
	e := &Executor{publisher: nil}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}
	// Should not panic.
	e.publishEvent(context.Background(), run, map[string]any{"from": "executing", "to": "completed"})
}

func TestPublishEvent_Success(t *testing.T) {
	t.Parallel()
	var captured []byte
	var capturedChannel string
	pub := &mockWorkerPublisher{
		publishFn: func(_ context.Context, channel string, data []byte) error {
			capturedChannel = channel
			captured = append([]byte{}, data...)
			return nil
		},
	}
	e := &Executor{publisher: pub, logger: noopLogger()}
	run := &domain.JobRun{ID: "run-42", JobID: "job-1", ProjectID: "proj-1"}

	e.publishEvent(context.Background(), run, map[string]any{"from": "executing", "to": "completed"})

	if pub.calls.Load() != 1 {
		t.Fatalf("expected 1 publish call, got %d", pub.calls.Load())
	}
	if capturedChannel != "run:run-42" {
		t.Fatalf("expected channel run:run-42, got %s", capturedChannel)
	}
	var event map[string]any
	if err := json.Unmarshal(captured, &event); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}
	if event["type"] != "status_change" {
		t.Fatalf("expected type=status_change, got %v", event["type"])
	}
	if event["run_id"] != "run-42" {
		t.Fatalf("expected run_id=run-42, got %v", event["run_id"])
	}
	if event["from"] != "executing" || event["to"] != "completed" {
		t.Fatalf("expected from=executing, to=completed, got from=%v, to=%v", event["from"], event["to"])
	}
}

func TestPublishEvent_PublishError(t *testing.T) {
	t.Parallel()
	pub := &mockWorkerPublisher{
		publishFn: func(_ context.Context, _ string, _ []byte) error {
			return errors.New("redis down")
		},
	}
	e := &Executor{publisher: pub, logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}

	// Should not panic, should log error.
	e.publishEvent(context.Background(), run, map[string]any{"from": "executing", "to": "failed"})

	if pub.calls.Load() != 1 {
		t.Fatalf("expected 1 publish call, got %d", pub.calls.Load())
	}
}

// notifyWorkflowCallback tests.

func TestNotifyWorkflowCallback_NilCallback(t *testing.T) {
	t.Parallel()
	e := &Executor{workflowCallback: nil}
	run := &domain.JobRun{ID: "run-1"}
	// Should not panic.
	e.notifyWorkflowCallback(context.Background(), run)
}

func TestNotifyWorkflowCallback_Success(t *testing.T) {
	t.Parallel()
	var capturedRunID string
	cb := &mockWorkflowCallback{
		onTerminalFn: func(_ context.Context, run *domain.JobRun) error {
			capturedRunID = run.ID
			return nil
		},
	}
	e := &Executor{workflowCallback: cb, logger: noopLogger()}
	run := &domain.JobRun{ID: "run-99", JobID: "job-1"}

	e.notifyWorkflowCallback(context.Background(), run)

	if cb.calls.Load() != 1 {
		t.Fatalf("expected 1 callback call, got %d", cb.calls.Load())
	}
	if capturedRunID != "run-99" {
		t.Fatalf("expected run_id=run-99, got %s", capturedRunID)
	}
}

func TestNotifyWorkflowCallback_Error(t *testing.T) {
	t.Parallel()
	cb := &mockWorkflowCallback{
		onTerminalFn: func(_ context.Context, _ *domain.JobRun) error {
			return errors.New("callback failed")
		},
	}
	e := &Executor{workflowCallback: cb, logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1"}

	// Should not panic, should log error.
	e.notifyWorkflowCallback(context.Background(), run)

	if cb.calls.Load() != 1 {
		t.Fatalf("expected 1 callback call, got %d", cb.calls.Load())
	}
}

// resolveExecutionPolicy tests.

func TestResolveExecutionPolicy_NoWorkflowStep(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{}
	e := &Executor{store: ms, logger: noopLogger()}

	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: ""}
	fallback := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	got, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.maxAttempts != 3 || got.timeoutSecs != 30 {
		t.Fatalf("expected fallback policy, got maxAttempts=%d, timeoutSecs=%d", got.maxAttempts, got.timeoutSecs)
	}
}

func TestResolveExecutionPolicy_StepRunNotFound(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return nil, nil
		},
	}
	e := &Executor{store: ms, logger: noopLogger()}

	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: "wsr-missing"}
	fallback := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	got, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.maxAttempts != 3 {
		t.Fatalf("expected fallback maxAttempts=3, got %d", got.maxAttempts)
	}
}

func TestResolveExecutionPolicy_StepRunError(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return nil, errors.New("db error")
		},
	}
	e := &Executor{store: ms, logger: noopLogger()}

	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: "wsr-1"}
	fallback := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	_, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveExecutionPolicy_WorkflowRunNotFound(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "wsr-1", WorkflowRunID: "wr-missing", StepRef: "step-a"}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return nil, nil
		},
	}
	e := &Executor{store: ms, logger: noopLogger()}

	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: "wsr-1"}
	fallback := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	got, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.maxAttempts != 3 {
		t.Fatalf("expected fallback, got maxAttempts=%d", got.maxAttempts)
	}
}

func TestResolveExecutionPolicy_WorkflowRunError(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "wsr-1", WorkflowRunID: "wr-1", StepRef: "step-a"}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return nil, errors.New("db error")
		},
	}
	e := &Executor{store: ms, logger: noopLogger()}

	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: "wsr-1"}
	fallback := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	_, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveExecutionPolicy_OverridesFromStep(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "wsr-1", WorkflowRunID: "wr-1", StepRef: "step-a"}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 2}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{
					StepRef:               "step-other",
					RetryMaxAttempts:      10,
					TimeoutSecsOverride:   120,
					RetryBackoff:          "linear",
					RetryInitialDelaySecs: 5,
					RetryMaxDelaySecs:     600,
				},
				{
					StepRef:               "step-a",
					RetryMaxAttempts:      7,
					TimeoutSecsOverride:   90,
					RetryBackoff:          domain.RetryBackoffFixed,
					RetryInitialDelaySecs: 10,
					RetryMaxDelaySecs:     300,
				},
			}, nil
		},
	}
	e := &Executor{store: ms, logger: noopLogger()}

	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: "wsr-1"}
	fallback := executionPolicy{
		maxAttempts:      3,
		timeoutSecs:      30,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 1,
		retryMaxSecs:     3600,
	}

	got, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testutil.AssertEqual(t, got, executionPolicy{
		maxAttempts:      7,
		timeoutSecs:      90,
		retryBackoff:     domain.RetryBackoffFixed,
		retryInitialSecs: 10,
		retryMaxSecs:     300,
	}, cmp.AllowUnexported(executionPolicy{}))
}

func TestResolveExecutionPolicy_StepNotFoundInList(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "wsr-1", WorkflowRunID: "wr-1", StepRef: "step-missing"}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "step-a", RetryMaxAttempts: 10},
				{StepRef: "step-b", RetryMaxAttempts: 5},
			}, nil
		},
	}
	e := &Executor{store: ms, logger: noopLogger()}

	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: "wsr-1"}
	fallback := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	got, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Step not found → returns fallback unchanged.
	if got.maxAttempts != 3 {
		t.Fatalf("expected fallback maxAttempts=3, got %d", got.maxAttempts)
	}
}

func TestResolveExecutionPolicy_ListStepsError(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "wsr-1", WorkflowRunID: "wr-1", StepRef: "step-a"}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return nil, errors.New("db error")
		},
	}
	e := &Executor{store: ms, logger: noopLogger()}

	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: "wsr-1"}
	fallback := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	_, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResolveExecutionPolicy_PartialOverrides(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "wsr-1", WorkflowRunID: "wr-1", StepRef: "step-a"}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{
					StepRef:          "step-a",
					RetryMaxAttempts: 5,
					// Other fields zero — should NOT override fallback.
				},
			}, nil
		},
	}
	e := &Executor{store: ms, logger: noopLogger()}

	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: "wsr-1"}
	fallback := executionPolicy{
		maxAttempts:      3,
		timeoutSecs:      60,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 2,
		retryMaxSecs:     1800,
	}

	got, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	testutil.AssertEqual(t, got, executionPolicy{
		maxAttempts:      5,
		timeoutSecs:      60,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 2,
		retryMaxSecs:     1800,
	}, cmp.AllowUnexported(executionPolicy{}))
}

// SendWebhookWithClient tests.

func TestSendWebhookWithClient_EmptyURL(t *testing.T) {
	t.Parallel()
	job := &domain.Job{ID: "job-1", WebhookURL: ""}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}

	result := SendWebhookWithClient(context.Background(), http.DefaultClient, job, run, 3)
	if !result.Delivered {
		t.Fatal("expected delivered=true for empty URL")
	}
}

func TestSendWebhookWithClient_SuccessFirstAttempt(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type=application/json, got %s", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-Run-ID") != "run-1" {
			t.Errorf("expected X-Run-ID=run-1, got %s", r.Header.Get("X-Run-ID"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{ID: "job-1", WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusCompleted, Attempt: 1}

	result := SendWebhookWithClient(context.Background(), srv.Client(), job, run, 3)
	if !result.Delivered {
		t.Fatalf("expected delivered=true, got error: %s", result.Error)
	}
	if result.StatusCode != 200 {
		t.Fatalf("expected status=200, got %d", result.StatusCode)
	}
	if called.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", called.Load())
	}
}

func TestSendWebhookWithClient_ClientErrorNoRetry(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	job := &domain.Job{ID: "job-1", WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}

	result := SendWebhookWithClient(context.Background(), srv.Client(), job, run, 3)
	if result.Delivered {
		t.Fatal("expected not delivered for 400")
	}
	if result.StatusCode != 400 {
		t.Fatalf("expected status=400, got %d", result.StatusCode)
	}
	if called.Load() != 1 {
		t.Fatalf("expected 1 call (no retry on 4xx), got %d", called.Load())
	}
}

func TestSendWebhookWithClient_ServerErrorRetries(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		c := called.Add(1)
		if c >= 2 {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	job := &domain.Job{ID: "job-1", WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}

	// Use short timeout to keep test fast.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result := SendWebhookWithClient(ctx, srv.Client(), job, run, 3)
	if !result.Delivered {
		t.Fatalf("expected delivered on second attempt, got error: %s", result.Error)
	}
	if called.Load() != 2 {
		t.Fatalf("expected 2 calls (1 failure + 1 success), got %d", called.Load())
	}
}

func TestSendWebhookWithClient_ContextCanceled(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	job := &domain.Job{ID: "job-1", WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before retries can proceed.
	cancel()

	result := SendWebhookWithClient(ctx, srv.Client(), job, run, 3)
	if result.Delivered {
		t.Fatal("expected not delivered when context canceled")
	}
}

func TestSendWebhookWithClient_DefaultMaxAttempts(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{ID: "job-1", WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}

	// maxAttempts=0 should default to 3.
	result := SendWebhookWithClient(context.Background(), srv.Client(), job, run, 0)
	if !result.Delivered {
		t.Fatalf("expected delivered=true, got error: %s", result.Error)
	}
}

func TestSendWebhookWithClient_HMACSignature(t *testing.T) {
	t.Parallel()
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Webhook-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{ID: "job-1", WebhookURL: srv.URL, WebhookSecret: "test-secret-key"}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusCompleted}

	result := SendWebhookWithClient(context.Background(), srv.Client(), job, run, 1)
	if !result.Delivered {
		t.Fatalf("expected delivered=true, got error: %s", result.Error)
	}
	if gotSig == "" {
		t.Fatal("expected X-Webhook-Signature header to be set")
	}
	if len(gotSig) < 10 || gotSig[:7] != "sha256=" {
		t.Fatalf("expected signature to start with sha256=, got %s", gotSig)
	}
}

func TestSendWebhookWithClient_NoSignatureWithoutSecret(t *testing.T) {
	t.Parallel()
	var gotSig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Webhook-Signature")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{ID: "job-1", WebhookURL: srv.URL, WebhookSecret: ""}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}

	result := SendWebhookWithClient(context.Background(), srv.Client(), job, run, 1)
	if !result.Delivered {
		t.Fatalf("expected delivered=true, got error: %s", result.Error)
	}
	if gotSig != "" {
		t.Fatalf("expected no signature header, got %s", gotSig)
	}
}

func TestSendWebhookWithClient_PayloadContent(t *testing.T) {
	t.Parallel()
	var received WebhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("failed to decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{ID: "job-1", WebhookURL: srv.URL}
	run := &domain.JobRun{
		ID:        "run-42",
		JobID:     "job-1",
		ProjectID: "proj-1",
		Status:    domain.StatusCompleted,
		Attempt:   3,
		Result:    json.RawMessage(`{"output":"done"}`),
		Error:     "some error",
	}

	result := SendWebhookWithClient(context.Background(), srv.Client(), job, run, 1)
	if !result.Delivered {
		t.Fatalf("expected delivered=true, got error: %s", result.Error)
	}
	if received.RunID != "run-42" {
		t.Fatalf("expected run_id=run-42, got %s", received.RunID)
	}
	if received.JobID != "job-1" {
		t.Fatalf("expected job_id=job-1, got %s", received.JobID)
	}
	if received.ProjectID != "proj-1" {
		t.Fatalf("expected project_id=proj-1, got %s", received.ProjectID)
	}
	if received.Status != "completed" {
		t.Fatalf("expected status=completed, got %s", received.Status)
	}
	if received.Attempt != 3 {
		t.Fatalf("expected attempt=3, got %d", received.Attempt)
	}
	if received.Error != "some error" {
		t.Fatalf("expected error='some error', got %s", received.Error)
	}
}

func TestSendWebhookWithClient_NetworkError(t *testing.T) {
	t.Parallel()
	job := &domain.Job{ID: "job-1", WebhookURL: "http://localhost:1"}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}

	// Use a client with very short timeout.
	client := &http.Client{Timeout: 100 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := SendWebhookWithClient(ctx, client, job, run, 1)
	if result.Delivered {
		t.Fatal("expected not delivered for network error")
	}
	if result.Error == "" {
		t.Fatal("expected error message for network error")
	}
}

// dispatchToEndpoint tests.

func TestDispatchToEndpoint_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Run-ID") != "run-1" {
			t.Errorf("expected X-Run-ID=run-1, got %s", r.Header.Get("X-Run-ID"))
		}
		if r.Header.Get("X-Job-ID") != "job-1" {
			t.Errorf("expected X-Job-ID=job-1, got %s", r.Header.Get("X-Job-ID"))
		}
		if r.Header.Get("X-Attempt") != "1" {
			t.Errorf("expected X-Attempt=1, got %s", r.Header.Get("X-Attempt"))
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"result":"ok"}`)
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client(), logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Payload: json.RawMessage(`{"input":"data"}`)}

	result, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != `{"result":"ok"}` {
		t.Fatalf("expected {\"result\":\"ok\"}, got %s", string(result))
	}
}

func TestDispatchToEndpoint_WithExtraHeaders(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Secret-MY_KEY") != "secret-value" {
			t.Errorf("expected X-Secret-MY_KEY=secret-value, got %s", r.Header.Get("X-Secret-MY_KEY"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client(), logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}
	headers := map[string]string{"X-Secret-MY_KEY": "secret-value"}

	_, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDispatchToEndpoint_NonOKStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "service down")
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client(), logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	_, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
	var endpointErr *domain.EndpointError
	if !errors.As(err, &endpointErr) {
		t.Fatalf("expected EndpointError, got %T: %v", err, err)
	}
	if endpointErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status=503, got %d", endpointErr.StatusCode)
	}
}

func TestDispatchToEndpoint_EmptyBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client(), logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	result, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for empty body, got %s", string(result))
	}
}

func TestDispatchToEndpoint_InvalidURL(t *testing.T) {
	t.Parallel()
	e := &Executor{httpClient: http.DefaultClient, logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	_, err := e.dispatchToEndpoint(context.Background(), "://invalid", run, nil)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// recordRunTransition tests.

func TestRecordRunTransition_NilMetrics(t *testing.T) {
	t.Parallel()
	e := &Executor{metrics: nil}
	// Should not panic.
	e.recordRunTransition(context.Background(), domain.StatusExecuting, domain.StatusCompleted)
}

// handleSuccess integration (through execute) — with publish + callback.

func TestExecutor_HandleSuccess_PublishesAndCallsBack(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"done":true}`)
	}))
	defer srv.Close()

	pub := &mockWorkerPublisher{}
	cb := &mockWorkflowCallback{}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return testJob(srv.URL, 3, 30), nil
		},
	}

	pool := NewPool(4)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             &mockExecQueue{},
		Store:             ms,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
		HTTPClient:        srv.Client(),
		Publisher:         pub,
		WorkflowCallback:  cb,
	})

	run := testRun(1)
	exec.execute(context.Background(), run)

	// Wait briefly for async webhook submit.
	time.Sleep(200 * time.Millisecond)

	if pub.calls.Load() < 1 {
		t.Fatalf("expected at least 1 publish call (status change), got %d", pub.calls.Load())
	}
	if cb.calls.Load() != 1 {
		t.Fatalf("expected 1 callback call, got %d", cb.calls.Load())
	}

	updates := ms.statusUpdates()
	found := false
	for _, u := range updates {
		if u.to == domain.StatusCompleted {
			found = true
		}
	}
	if !found {
		t.Fatal("expected status transition to completed")
	}
}

// handleFailure with publish + callback.

func TestExecutor_HandleFailure_PublishesAndCallsBack(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "error")
	}))
	defer srv.Close()

	pub := &mockWorkerPublisher{}
	cb := &mockWorkflowCallback{}

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return testJob(srv.URL, 1, 30), nil // maxAttempts=1 → immediate failure.
		},
	}

	pool := NewPool(4)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             &mockExecQueue{},
		Store:             ms,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
		HTTPClient:        srv.Client(),
		Publisher:         pub,
		WorkflowCallback:  cb,
	})

	run := testRun(1)
	exec.execute(context.Background(), run)

	time.Sleep(200 * time.Millisecond)

	if pub.calls.Load() < 1 {
		t.Fatalf("expected at least 1 publish call, got %d", pub.calls.Load())
	}
	if cb.calls.Load() != 1 {
		t.Fatalf("expected 1 callback call, got %d", cb.calls.Load())
	}

	updates := ms.statusUpdates()
	found := false
	for _, u := range updates {
		if u.to == domain.StatusFailed {
			found = true
		}
	}
	if !found {
		t.Fatal("expected status transition to failed")
	}
}

// Workflow step run → execution policy override through execute.

func TestExecutor_Execute_WithWorkflowPolicyOverride(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	ms := &mockExecutorStore{
		getJobFn: func(_ context.Context, _ string) (*domain.Job, error) {
			return testJob(srv.URL, 3, 30), nil
		},
		getWorkflowStepRunFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{ID: "wsr-1", WorkflowRunID: "wr-1", StepRef: "step-a"}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", WorkflowVersion: 1}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{
					StepRef:             "step-a",
					RetryMaxAttempts:    10,
					TimeoutSecsOverride: 120,
				},
			}, nil
		},
	}

	pool := NewPool(4)
	t.Cleanup(func() { _ = pool.Shutdown(context.Background()) })

	exec := NewExecutor(ExecutorConfig{
		Pool:              pool,
		Queue:             &mockExecQueue{},
		Store:             ms,
		PollInterval:      time.Millisecond,
		HeartbeatInterval: time.Hour,
		HTTPClient:        srv.Client(),
	})

	run := testRun(1)
	run.WorkflowStepRunID = "wsr-1"
	exec.execute(context.Background(), run)

	updates := ms.statusUpdates()
	found := false
	for _, u := range updates {
		if u.to == domain.StatusCompleted {
			found = true
		}
	}
	if !found {
		t.Fatal("expected status transition to completed")
	}
}

// noopLogger returns a logger that discards all output.
func noopLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
