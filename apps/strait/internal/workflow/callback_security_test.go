package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// TestCallback_StepOutputInjection verifies that malicious output in a step
// result is stored verbatim without being interpreted or causing corruption.
func TestCallback_StepOutputInjection(t *testing.T) {
	t.Parallel()

	maliciousPayloads := []string{
		`{"__proto__":{"admin":true}}`,
		`{"constructor":{"prototype":{"isAdmin":true}}}`,
		`{"$where":"sleep(5000)"}`,
		`{"key":"val\u0000\u0000null bytes"}`,
		`{"nested":{"<script>alert(1)</script>":"xss"}}`,
	}

	for _, payload := range maliciousPayloads {
		t.Run(payload[:min(len(payload), 40)], func(t *testing.T) {
			t.Parallel()

			var storedOutput json.RawMessage
			ms := &mockCallbackStore{
				getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
					return &domain.WorkflowStepRun{
						ID:            "sr-1",
						WorkflowRunID: "wr-1",
						StepRef:       "step-a",
						Status:        domain.StepRunning,
					}, nil
				},
				getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
					return &domain.WorkflowRun{
						ID:         "wr-1",
						WorkflowID: "wf-1",
						Status:     domain.WfStatusRunning,
					}, nil
				},
				listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
					return []domain.WorkflowStep{{StepRef: "step-a"}}, nil
				},
				updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, fields map[string]any) error {
					if out, ok := fields["output"].(json.RawMessage); ok {
						storedOutput = out
					}
					return nil
				},
				incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
					return nil, nil
				},
				countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
					return 0, nil
				},
				listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
					return nil, nil
				},
				updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
					return nil
				},
			}

			cb := newTestCallback(ms)
			run := &domain.JobRun{
				ID:                "jr-1",
				JobID:             "job-1",
				Status:            domain.StatusCompleted,
				Result:            json.RawMessage(payload),
				WorkflowStepRunID: "sr-1",
			}
			if err := cb.OnJobRunTerminal(context.Background(), run); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(storedOutput) != payload {
				t.Fatalf("output mutated: got %s, want %s", storedOutput, payload)
			}
		})
	}
}

// TestCallback_ConcurrentCallbacks verifies that multiple steps completing
// simultaneously do not corrupt shared workflow state.
func TestCallback_ConcurrentCallbacks(t *testing.T) {
	t.Parallel()

	var updateCalls atomic.Int32
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, jobRunID string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:            "sr-" + jobRunID,
				WorkflowRunID: "wr-1",
				StepRef:       "step-" + jobRunID,
				Status:        domain.StepRunning,
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{
				ID:         "wr-1",
				WorkflowID: "wf-1",
				Status:     domain.WfStatusRunning,
			}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{
				{StepRef: "step-jr-1"},
				{StepRef: "step-jr-2"},
				{StepRef: "step-jr-3"},
			}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
			updateCalls.Add(1)
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 1, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
	}

	cb := newTestCallback(ms)
	var wg sync.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := "jr-" + strings.Repeat("x", idx)
			run := &domain.JobRun{
				ID:                id,
				JobID:             "job-1",
				Status:            domain.StatusCompleted,
				Result:            json.RawMessage(`{"ok":true}`),
				WorkflowStepRunID: "sr-" + id,
			}
			_ = cb.OnJobRunTerminal(context.Background(), run)
		}(i)
	}
	wg.Wait()

	if got := updateCalls.Load(); got < 3 {
		t.Fatalf("expected at least 3 status updates, got %d", got)
	}
}

// TestCallback_OrphanedCallback verifies that a callback for a non-existent
// workflow run returns an error rather than panicking.
func TestCallback_OrphanedCallback(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:            "sr-1",
				WorkflowRunID: "wr-missing",
				StepRef:       "step-a",
				Status:        domain.StepRunning,
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return nil, nil // Not found.
		},
	}

	cb := newTestCallback(ms)
	run := &domain.JobRun{
		ID:                "jr-1",
		JobID:             "job-1",
		Status:            domain.StatusCompleted,
		WorkflowStepRunID: "sr-1",
	}
	err := cb.OnJobRunTerminal(context.Background(), run)
	if err == nil {
		t.Fatal("expected error for orphaned callback, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' in error, got: %v", err)
	}
}

// TestCallback_TerminalWorkflowCallback verifies that a callback for a step in
// a completed workflow does not advance the workflow further.
func TestCallback_TerminalWorkflowCallback(t *testing.T) {
	t.Parallel()

	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:            "sr-1",
				WorkflowRunID: "wr-1",
				StepRef:       "step-a",
				Status:        domain.StepCompleted, // Already terminal.
			}, nil
		},
	}

	cb := newTestCallback(ms)
	run := &domain.JobRun{
		ID:                "jr-1",
		JobID:             "job-1",
		Status:            domain.StatusCompleted,
		WorkflowStepRunID: "sr-1",
	}
	// Should return nil since step is already terminal.
	if err := cb.OnJobRunTerminal(context.Background(), run); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestCallback_HugeStepOutput verifies that a 10MB step output is handled
// without error.
func TestCallback_HugeStepOutput(t *testing.T) {
	t.Parallel()

	// Build a ~10MB JSON payload.
	bigValue := strings.Repeat("x", 10*1024*1024)
	hugeJSON, err := json.Marshal(map[string]string{"data": bigValue})
	if err != nil {
		t.Fatalf("failed to marshal huge payload: %v", err)
	}

	var storedSize int
	ms := &mockCallbackStore{
		getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
			return &domain.WorkflowStepRun{
				ID:            "sr-1",
				WorkflowRunID: "wr-1",
				StepRef:       "step-a",
				Status:        domain.StepRunning,
			}, nil
		},
		getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
			return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
		},
		listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
			return []domain.WorkflowStep{{StepRef: "step-a"}}, nil
		},
		updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, fields map[string]any) error {
			if out, ok := fields["output"].(json.RawMessage); ok {
				storedSize = len(out)
			}
			return nil
		},
		incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
			return nil, nil
		},
		countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
			return 0, nil
		},
		listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
			return nil, nil
		},
		updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
			return nil
		},
	}

	cb := newTestCallback(ms)
	run := &domain.JobRun{
		ID:                "jr-1",
		JobID:             "job-1",
		Status:            domain.StatusCompleted,
		Result:            json.RawMessage(hugeJSON),
		WorkflowStepRunID: "sr-1",
	}
	if err := cb.OnJobRunTerminal(context.Background(), run); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if storedSize < 10*1024*1024 {
		t.Fatalf("expected stored output >= 10MB, got %d bytes", storedSize)
	}
}

// FuzzCallbackOutput fuzzes the step output JSON to ensure the callback
// does not panic on arbitrary input.
func FuzzCallbackOutput(f *testing.F) {
	f.Add([]byte(`{"ok":true}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte{0x00, 0xff, 0xfe})

	f.Fuzz(func(t *testing.T, data []byte) {
		ms := &mockCallbackStore{
			getStepRunByJobRunIDFn: func(_ context.Context, _ string) (*domain.WorkflowStepRun, error) {
				return &domain.WorkflowStepRun{
					ID:            "sr-1",
					WorkflowRunID: "wr-1",
					StepRef:       "step-a",
					Status:        domain.StepRunning,
				}, nil
			},
			getWorkflowRunFn: func(_ context.Context, _ string) (*domain.WorkflowRun, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", Status: domain.WfStatusRunning}, nil
			},
			listStepsByWorkflowVerFn: func(_ context.Context, _ string, _ int) ([]domain.WorkflowStep, error) {
				return []domain.WorkflowStep{{StepRef: "step-a"}}, nil
			},
			updateStepRunStatusFn: func(_ context.Context, _ string, _ domain.StepRunStatus, _ map[string]any) error {
				return nil
			},
			incrementStepDepsFn: func(_ context.Context, _ string, _ string) ([]store.StepDepResult, error) {
				return nil, nil
			},
			countNonTerminalStepRunsFn: func(_ context.Context, _ string) (int, error) {
				return 0, nil
			},
			listFailedStepRunRefsFn: func(_ context.Context, _ string) ([]string, error) {
				return nil, nil
			},
			updateWorkflowRunStatusFn: func(_ context.Context, _ string, _, _ domain.WorkflowRunStatus, _ map[string]any) error {
				return nil
			},
		}

		cb := NewStepCallback(ms, NewWorkflowEngine(&mockEngineStore{}, &mockEngineQueue{}, slog.Default()), slog.Default())
		run := &domain.JobRun{
			ID:                "jr-1",
			JobID:             "job-1",
			Status:            domain.StatusCompleted,
			Result:            json.RawMessage(data),
			WorkflowStepRunID: "sr-1",
		}
		// Must not panic regardless of output content.
		_ = cb.OnJobRunTerminal(context.Background(), run)
	})
}

// Suppress unused import warnings.
var _ = time.Now
var _ = errors.New
