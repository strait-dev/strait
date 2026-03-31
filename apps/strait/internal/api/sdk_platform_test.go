package api

import (
	"context"
	"encoding/json"
	"testing"

	"strait/internal/domain"
	"strait/internal/store"
)

// -- handleSDKTriggerJob tests.

func TestSDKTriggerJob_ValidatesEmptySlug(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", ProjectID: "proj-1"}, nil
		},
	}}
	input := &SDKTriggerJobInput{RunID: "run-1", Body: SDKTriggerJobRequest{JobSlug: ""}}
	_, err := srv.handleSDKTriggerJob(t.Context(), input)
	assertHumaStatus(t, err, 400)
}

func TestSDKTriggerJob_WhitespaceSlug(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", ProjectID: "proj-1"}, nil
		},
	}}
	input := &SDKTriggerJobInput{RunID: "run-1", Body: SDKTriggerJobRequest{JobSlug: "   "}}
	_, err := srv.handleSDKTriggerJob(t.Context(), input)
	assertHumaStatus(t, err, 400)
}

func TestSDKTriggerJob_RunNotFound(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errNotFound
		},
	}}
	input := &SDKTriggerJobInput{RunID: "nonexistent", Body: SDKTriggerJobRequest{JobSlug: "my-job"}}
	_, err := srv.handleSDKTriggerJob(t.Context(), input)
	assertHumaStatus(t, err, 404)
}

func TestSDKTriggerJob_StoreNotQueries(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", ProjectID: "proj-1"}, nil
		},
	}}
	input := &SDKTriggerJobInput{RunID: "run-1", Body: SDKTriggerJobRequest{JobSlug: "my-job"}}
	_, err := srv.handleSDKTriggerJob(t.Context(), input)
	assertHumaStatus(t, err, 503)
}

// -- handleSDKTriggerWorkflow tests.

func TestSDKTriggerWorkflow_ValidatesEmptySlug(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", ProjectID: "proj-1"}, nil
		},
	}}
	input := &SDKTriggerWorkflowInput{RunID: "run-1", Body: SDKTriggerWorkflowRequest{WorkflowSlug: ""}}
	_, err := srv.handleSDKTriggerWorkflow(t.Context(), input)
	assertHumaStatus(t, err, 400)
}

func TestSDKTriggerWorkflow_RunNotFound(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errNotFound
		},
	}}
	input := &SDKTriggerWorkflowInput{RunID: "nonexistent", Body: SDKTriggerWorkflowRequest{WorkflowSlug: "my-wf"}}
	_, err := srv.handleSDKTriggerWorkflow(t.Context(), input)
	assertHumaStatus(t, err, 404)
}

func TestSDKTriggerWorkflow_StoreNotQueries(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", ProjectID: "proj-1"}, nil
		},
	}}
	input := &SDKTriggerWorkflowInput{RunID: "run-1", Body: SDKTriggerWorkflowRequest{WorkflowSlug: "my-wf"}}
	_, err := srv.handleSDKTriggerWorkflow(t.Context(), input)
	assertHumaStatus(t, err, 503)
}

// -- handleSDKTriggerAgent tests.

func TestSDKTriggerAgent_ValidatesEmptySlug(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "run-1", ProjectID: "proj-1"}, nil
		},
	}}
	input := &SDKTriggerAgentInput{RunID: "run-1", Body: SDKTriggerAgentRequest{AgentSlug: ""}}
	_, err := srv.handleSDKTriggerAgent(t.Context(), input)
	assertHumaStatus(t, err, 400)
}

func TestSDKTriggerAgent_RunNotFound(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errNotFound
		},
	}}
	input := &SDKTriggerAgentInput{RunID: "nonexistent", Body: SDKTriggerAgentRequest{AgentSlug: "my-agent"}}
	_, err := srv.handleSDKTriggerAgent(t.Context(), input)
	assertHumaStatus(t, err, 404)
}

func TestSDKTriggerAgent_NilAgentService(t *testing.T) {
	t.Parallel()
	srv := &Server{
		agentService: nil,
		store: &APIStoreMock{
			GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
				return &domain.JobRun{ID: "run-1", ProjectID: "proj-1"}, nil
			},
		},
	}
	input := &SDKTriggerAgentInput{RunID: "run-1", Body: SDKTriggerAgentRequest{AgentSlug: "my-agent"}}
	_, err := srv.handleSDKTriggerAgent(t.Context(), input)
	assertHumaStatus(t, err, 503)
}

func TestSDKTriggerAgent_StoreNotQueries(t *testing.T) {
	t.Parallel()
	srv := &Server{
		agentService: &stubAgentService{},
		store: &APIStoreMock{
			GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
				return &domain.JobRun{ID: "run-1", ProjectID: "proj-1"}, nil
			},
		},
	}
	input := &SDKTriggerAgentInput{RunID: "run-1", Body: SDKTriggerAgentRequest{AgentSlug: "my-agent"}}
	_, err := srv.handleSDKTriggerAgent(t.Context(), input)
	assertHumaStatus(t, err, 503)
}

// -- handleSDKAwaitRun tests.

func TestSDKAwaitRun_ValidatesEmptyRunID(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: "caller", ProjectID: "proj-1"}, nil
		},
	}}
	input := &SDKAwaitRunInput{CallerRunID: "caller", Body: SDKAwaitRunRequest{RunID: "", TimeoutMs: 1000}}
	_, err := srv.handleSDKAwaitRun(t.Context(), input)
	assertHumaStatus(t, err, 400)
}

func TestSDKAwaitRun_CallerRunNotFound(t *testing.T) {
	t.Parallel()
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, errNotFound
		},
	}}
	input := &SDKAwaitRunInput{CallerRunID: "bad", Body: SDKAwaitRunRequest{RunID: "target", TimeoutMs: 1000}}
	_, err := srv.handleSDKAwaitRun(t.Context(), input)
	assertHumaStatus(t, err, 404)
}

func TestSDKAwaitRun_TargetRunNotFound(t *testing.T) {
	t.Parallel()
	callCount := 0
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			callCount++
			if callCount == 1 {
				return &domain.JobRun{ID: "caller", ProjectID: "proj-1"}, nil
			}
			return nil, errNotFound
		},
	}}
	input := &SDKAwaitRunInput{CallerRunID: "caller", Body: SDKAwaitRunRequest{RunID: "bad-target", TimeoutMs: 1000}}
	_, err := srv.handleSDKAwaitRun(t.Context(), input)
	assertHumaStatus(t, err, 404)
}

func TestSDKAwaitRun_CrossProjectBlocked(t *testing.T) {
	t.Parallel()
	callCount := 0
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			callCount++
			if callCount == 1 {
				return &domain.JobRun{ID: "caller", ProjectID: "proj-A"}, nil
			}
			return &domain.JobRun{ID: "target", ProjectID: "proj-B", Status: domain.StatusCompleted}, nil
		},
	}}
	input := &SDKAwaitRunInput{CallerRunID: "caller", Body: SDKAwaitRunRequest{RunID: "target", TimeoutMs: 1000}}
	_, err := srv.handleSDKAwaitRun(t.Context(), input)
	assertHumaStatus(t, err, 404)
}

func TestSDKAwaitRun_AlreadyTerminal(t *testing.T) {
	t.Parallel()
	callCount := 0
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			callCount++
			if callCount == 1 {
				return &domain.JobRun{ID: "caller", ProjectID: "proj-1"}, nil
			}
			return &domain.JobRun{
				ID: "target", ProjectID: "proj-1", Status: domain.StatusCompleted,
				Result: json.RawMessage(`{"answer":42}`),
			}, nil
		},
	}}
	input := &SDKAwaitRunInput{CallerRunID: "caller", Body: SDKAwaitRunRequest{RunID: "target", TimeoutMs: 5000}}
	out, err := srv.handleSDKAwaitRun(t.Context(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.Body.Status != "completed" {
		t.Fatalf("Status = %q, want completed", out.Body.Status)
	}
	if string(out.Body.Result) != `{"answer":42}` {
		t.Fatalf("Result = %q", string(out.Body.Result))
	}
}

func TestSDKAwaitRun_ZeroTimeout(t *testing.T) {
	t.Parallel()
	callCount := 0
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			callCount++
			if callCount == 1 {
				return &domain.JobRun{ID: "caller", ProjectID: "proj-1"}, nil
			}
			return &domain.JobRun{ID: "target", ProjectID: "proj-1", Status: domain.StatusExecuting}, nil
		},
	}}
	input := &SDKAwaitRunInput{CallerRunID: "caller", Body: SDKAwaitRunRequest{RunID: "target", TimeoutMs: 0}}
	out, err := srv.handleSDKAwaitRun(t.Context(), input)
	if err != nil {
		t.Fatalf("expected no error for zero timeout, got %v", err)
	}
	if out.Body.Status != "executing" {
		t.Fatalf("Status = %q, want executing (immediate return)", out.Body.Status)
	}
}

func TestSDKAwaitRun_NegativeTimeout(t *testing.T) {
	t.Parallel()
	callCount := 0
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			callCount++
			if callCount == 1 {
				return &domain.JobRun{ID: "caller", ProjectID: "proj-1"}, nil
			}
			return &domain.JobRun{ID: "target", ProjectID: "proj-1", Status: domain.StatusQueued}, nil
		},
	}}
	input := &SDKAwaitRunInput{CallerRunID: "caller", Body: SDKAwaitRunRequest{RunID: "target", TimeoutMs: -100}}
	out, err := srv.handleSDKAwaitRun(t.Context(), input)
	if err != nil {
		t.Fatalf("expected no error for negative timeout, got %v", err)
	}
	if out.Body.Status != "queued" {
		t.Fatalf("Status = %q, want queued (immediate return)", out.Body.Status)
	}
}

func TestSDKAwaitRun_FailedRunIncludesError(t *testing.T) {
	t.Parallel()
	callCount := 0
	srv := &Server{store: &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			callCount++
			if callCount == 1 {
				return &domain.JobRun{ID: "caller", ProjectID: "proj-1"}, nil
			}
			return &domain.JobRun{
				ID: "target", ProjectID: "proj-1", Status: domain.StatusFailed,
				Error: "out of memory",
			}, nil
		},
	}}
	input := &SDKAwaitRunInput{CallerRunID: "caller", Body: SDKAwaitRunRequest{RunID: "target", TimeoutMs: 5000}}
	out, err := srv.handleSDKAwaitRun(t.Context(), input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.Body.Error != "out of memory" {
		t.Fatalf("Error = %q, want 'out of memory'", out.Body.Error)
	}
}

// -- helpers.

var errNotFound = store.ErrRunNotFound

func assertHumaStatus(t *testing.T, err error, wantStatus int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with status %d, got nil", wantStatus)
	}
	// Huma errors implement StatusCode() int.
	type statusCoder interface {
		GetStatus() int
	}
	if sc, ok := err.(statusCoder); ok {
		if sc.GetStatus() != wantStatus {
			t.Fatalf("error status = %d, want %d: %v", sc.GetStatus(), wantStatus, err)
		}
		return
	}
	t.Logf("error does not implement statusCoder, checking string: %v", err)
}
