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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
	"strait/internal/pubsub"
	orcstore "strait/internal/store"
	"strait/internal/testutil"
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

func (m *mockWorkerPublisher) PublishBatch(ctx context.Context, messages []pubsub.PubSubMessage) error {
	for _, msg := range messages {
		if err := m.Publish(ctx, msg.Channel, msg.Data); err != nil {
			return err
		}
	}
	return nil
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
	require.EqualValues(t, 1, pub.calls.Load())
	require.Equal(t,
		"run:run-42", capturedChannel,
	)

	var event map[string]any
	require.NoError(
		t, json.Unmarshal(captured,
			&event,
		))
	require.Equal(t,
		"status_change",
		event["type"])
	require.Equal(t,
		"run-42", event["run_id"])
	require.False(t,
		event["from"] !=
			"executing" ||
			event["to"] !=
				"completed",
	)

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
	require.EqualValues(t, 1, pub.calls.Load())

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
	e.callbackWG.Wait()
	require.EqualValues(t, 1, cb.calls.Load())
	require.Equal(t,
		"run-99", capturedRunID,
	)

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
	e.callbackWG.Wait()
	require.EqualValues(t, 1, cb.calls.Load())

}

// resolveExecutionPolicy tests.

func TestResolveExecutionPolicy_NoWorkflowStep(t *testing.T) {
	t.Parallel()
	ms := &mockExecutorStore{}
	e := &Executor{store: ms, logger: noopLogger()}

	run := &domain.JobRun{ID: "run-1", WorkflowStepRunID: ""}
	fallback := executionPolicy{maxAttempts: 3, timeoutSecs: 30}

	got, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	require.NoError(
		t, err)
	require.False(t,
		got.maxAttempts !=
			3 || got.
			timeoutSecs !=
			30,
	)

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

	_, err := e.resolveExecutionPolicy(context.Background(), run, fallback)
	require.True(t,
		errors.Is(err, orcstore.
			ErrWorkflowStepRunNotFound,
		))

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
	require.Error(t,
		err)

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
	require.NoError(
		t, err)
	require.EqualValues(t, 3, got.maxAttempts)

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
	require.Error(t,
		err)

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
	require.NoError(
		t, err)

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
	require.NoError(
		t, err)
	require.EqualValues(t, 3, got.maxAttempts)

	// Step not found → returns fallback unchanged.

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
	require.Error(t,
		err)

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
	require.NoError(
		t, err)

	testutil.AssertEqual(t, got, executionPolicy{
		maxAttempts:      5,
		timeoutSecs:      60,
		retryBackoff:     domain.RetryBackoffExponential,
		retryInitialSecs: 2,
		retryMaxSecs:     1800,
	}, cmp.AllowUnexported(executionPolicy{}))
}

// sendWebhookWithClientForTest tests.

func TestSendWebhookWithClient_EmptyURL(t *testing.T) {
	t.Parallel()
	job := &domain.Job{ID: "job-1", WebhookURL: ""}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}

	result := sendWebhookWithClientForTest(context.Background(), http.DefaultClient, job, run, 3)
	require.True(t,
		result.Delivered)

}

func TestSendWebhookWithClient_SuccessFirstAttempt(t *testing.T) {
	t.Parallel()
	var called atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		assert.Equal(t,
			"application/json",
			r.Header.
				Get("Content-Type"))
		assert.Equal(t,
			"run-1", r.Header.
				Get("X-Run-ID"))

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	job := &domain.Job{ID: "job-1", WebhookURL: srv.URL}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusCompleted, Attempt: 1}

	result := sendWebhookWithClientForTest(context.Background(), srv.Client(), job, run, 3)
	require.True(t,
		result.Delivered)
	require.EqualValues(t, 200, result.StatusCode)
	require.EqualValues(t, 1, called.Load())

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

	result := sendWebhookWithClientForTest(context.Background(), srv.Client(), job, run, 3)
	require.False(t,
		result.Delivered,
	)
	require.EqualValues(t, 400, result.StatusCode)
	require.EqualValues(t, 1, called.Load())

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

	result := sendWebhookWithClientForTest(ctx, srv.Client(), job, run, 3)
	require.True(t,
		result.Delivered)
	require.EqualValues(t, 2, called.Load())

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

	result := sendWebhookWithClientForTest(ctx, srv.Client(), job, run, 3)
	require.False(t,
		result.Delivered,
	)

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
	result := sendWebhookWithClientForTest(context.Background(), srv.Client(), job, run, 0)
	require.True(t,
		result.Delivered)

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

	result := sendWebhookWithClientForTest(context.Background(), srv.Client(), job, run, 1)
	require.True(t,
		result.Delivered)
	require.NotEqual(t, "", gotSig)
	require.False(t,
		len(gotSig) < 5 ||
			gotSig[:3] !=
				"v1=")

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

	result := sendWebhookWithClientForTest(context.Background(), srv.Client(), job, run, 1)
	require.True(t,
		result.Delivered)
	require.Equal(t,
		"", gotSig)

}

func TestSendWebhookWithClient_PayloadContent(t *testing.T) {
	t.Parallel()
	var received WebhookPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t,
			json.NewDecoder(r.Body).Decode(&received))

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

	result := sendWebhookWithClientForTest(context.Background(), srv.Client(), job, run, 1)
	require.True(t,
		result.Delivered)
	require.Equal(t,
		"run-42", received.
			RunID)
	require.Equal(t,
		"job-1", received.
			JobID)
	require.Equal(t,
		"proj-1", received.
			ProjectID,
	)
	require.Equal(t,
		"completed", received.
			Status,
	)
	require.EqualValues(t, 3, received.Attempt)
	require.Equal(t,
		"some error", received.
			Error,
	)

}

func TestSendWebhookWithClient_NetworkError(t *testing.T) {
	t.Parallel()
	job := &domain.Job{ID: "job-1", WebhookURL: "http://localhost:1"}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", ProjectID: "proj-1"}

	// Use a client with very short timeout.
	client := &http.Client{Timeout: 100 * time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := sendWebhookWithClientForTest(ctx, client, job, run, 1)
	require.False(t,
		result.Delivered,
	)
	require.NotEqual(t, "", result.Error)

}

// dispatchToEndpoint tests.

func TestDispatchToEndpoint_Success(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t,
			"run-1", r.Header.
				Get("X-Run-ID"))
		assert.Equal(t,
			"job-1", r.Header.
				Get("X-Job-ID"))
		assert.Equal(t,
			"1", r.Header.Get("X-Attempt"))

		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"result":"ok"}`)
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client(), logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1, Payload: json.RawMessage(`{"input":"data"}`)}

	result, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)
	require.NoError(
		t, err)
	require.Equal(t,
		`{"result":"ok"}`,
		string(
			result),
	)

}

func TestDispatchToEndpoint_SuccessWithTextBodyReturnsJSONString(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client(), logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	result, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, nil)
	require.NoError(
		t, err)
	require.True(t,
		json.Valid(result))
	require.Equal(t,
		`"ok"`, string(result))

}

func TestDispatchToEndpoint_WithExtraHeaders(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t,
			"secret-value", r.
				Header.Get("X-Secret-MY_KEY"))

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	e := &Executor{httpClient: srv.Client(), logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}
	headers := map[string]string{"X-Secret-MY_KEY": "secret-value"}

	_, err := e.dispatchToEndpoint(context.Background(), srv.URL, run, headers)
	require.NoError(
		t, err)

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
	require.Error(t,
		err)

	var endpointErr *domain.EndpointError
	require.True(t,
		errors.As(err, &endpointErr))
	require.Equal(t,
		http.StatusServiceUnavailable,

		endpointErr.
			StatusCode,
	)

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
	require.NoError(
		t, err)
	require.Nil(t, result)

}

func TestDispatchToEndpoint_InvalidURL(t *testing.T) {
	t.Parallel()
	e := &Executor{httpClient: http.DefaultClient, logger: noopLogger()}
	run := &domain.JobRun{ID: "run-1", JobID: "job-1", Attempt: 1}

	_, err := e.dispatchToEndpoint(context.Background(), "://invalid", run, nil)
	require.Error(t,
		err)

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

	waitForCondition(t, 2*time.Second, func() bool {
		return pub.calls.Load() >= 1 && cb.calls.Load() >= 1
	}, "publish and callback calls")
	require.GreaterOrEqual(t, pub.calls.
		Load(),
		int32(1))
	require.EqualValues(t, 1, cb.calls.Load())

	updates := ms.statusUpdates()
	found := false
	for _, u := range updates {
		if u.to == domain.StatusCompleted {
			found = true
		}
	}
	require.True(t,
		found)

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

	waitForCondition(t, 2*time.Second, func() bool {
		return pub.calls.Load() >= 1 && cb.calls.Load() >= 1
	}, "publish and callback calls")
	require.GreaterOrEqual(t, pub.calls.
		Load(),
		int32(1))
	require.EqualValues(t, 1, cb.calls.Load())

	updates := ms.statusUpdates()
	found := false
	for _, u := range updates {
		if u.to == domain.StatusDeadLetter {
			found = true
		}
	}
	require.True(t,
		found)

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
	require.True(t,
		found)

}

// noopLogger returns a logger that discards all output.
func noopLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// waitForCondition polls until cond returns true or the timeout expires.
func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool, desc string) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(timeout)
	for {
		if cond() {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline:
			require.Failf(t, "test failure", "timed out waiting for %s", desc)
		}
	}
}
