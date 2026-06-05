package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"
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
			require.NoError(t,
				cb.OnJobRunTerminal(context.
					Background(), run))
			require.Equal(t, payload,
				string(storedOutput))
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
	var wg conc.WaitGroup
	for i := 1; i <= 3; i++ {
		wg.Go(func() {
			id := "jr-" + strings.Repeat("x", i)
			run := &domain.JobRun{
				ID:                id,
				JobID:             "job-1",
				Status:            domain.StatusCompleted,
				Result:            json.RawMessage(`{"ok":true}`),
				WorkflowStepRunID: "sr-" + id,
			}
			_ = cb.OnJobRunTerminal(context.Background(), run)
		})
	}
	wg.Wait()
	require.GreaterOrEqual(t, updateCalls.
		Load(),
		int32(3))
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
	require.Error(t, err)
	require.Contains(t, err.
		Error(), "not found")
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
	require.NoError(t,
		cb.OnJobRunTerminal(context.
			Background(), run))

	// Should return nil since step is already terminal.
}

func TestEmitEventIfConfigured_DoesNotResolveForeignProjectEventKey(t *testing.T) {
	t.Parallel()

	var scopedLookupProject string
	var updateCalled atomic.Bool
	ms := &mockCallbackStore{
		getEventTriggerByEventKeyFn: func(_ context.Context, _ string) (*domain.EventTrigger, error) {
			return &domain.EventTrigger{
				ID:         "evt-foreign",
				EventKey:   "shared-key",
				ProjectID:  "proj-foreign",
				Status:     domain.EventTriggerStatusWaiting,
				SourceType: domain.EventSourceJobRun,
				JobRunID:   "foreign-run",
			}, nil
		},
		getEventTriggerByEventKeyProjectFn: func(_ context.Context, eventKey, projectID string) (*domain.EventTrigger, error) {
			scopedLookupProject = projectID
			if eventKey == "shared-key" && projectID == "proj-foreign" {
				return &domain.EventTrigger{
					ID:         "evt-foreign",
					EventKey:   eventKey,
					ProjectID:  projectID,
					Status:     domain.EventTriggerStatusWaiting,
					SourceType: domain.EventSourceJobRun,
					JobRunID:   "foreign-run",
				}, nil
			}
			return nil, nil
		},
		updateEventTriggerStatusFn: func(_ context.Context, _ string, _ string, _ json.RawMessage, _ *time.Time, _ string) error {
			updateCalled.Store(true)
			return nil
		},
		updateRunStatusFn: func(_ context.Context, _ string, _, _ domain.RunStatus, _ map[string]any) error {
			updateCalled.Store(true)
			return nil
		},
	}

	cb := newTestCallback(ms)
	cb.emitEventIfConfigured(
		context.Background(),
		&domain.WorkflowStepRun{ID: "sr-1", Output: json.RawMessage(`{"ok":true}`)},
		&domain.WorkflowStep{StepRef: "emitter", EventEmitKey: "shared-key"},
		&domain.WorkflowRun{ID: "wr-1", WorkflowID: "wf-1", ProjectID: "proj-1"},
	)
	require.Equal(t, "proj-1",
		scopedLookupProject,
	)
	require.False(t, updateCalled.
		Load())
}

// TestCallback_HugeStepOutput verifies that a 10MB step output is handled
// without error.
func TestCallback_HugeStepOutput(t *testing.T) {
	t.Parallel()

	// Build a ~10MB JSON payload.
	bigValue := strings.Repeat("x", 10*1024*1024)
	hugeJSON, err := json.Marshal(map[string]string{"data": bigValue})
	require.NoError(t,
		err)

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
	require.NoError(t,
		cb.OnJobRunTerminal(context.
			Background(), run))
	require.GreaterOrEqual(t, storedSize,
		10*1024*
			1024)
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
