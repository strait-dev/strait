package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

// extractPath deep nesting edge cases.

func TestExtractPath_FiveLevelDeep(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": map[string]any{
						"e": "five-deep",
					},
				},
			},
		},
	}
	got := extractPath(data, "a.b.c.d.e")
	assert.Equal(t,
		"five-deep", got)

}

func TestExtractPath_SixLevelDeep(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"l1": map[string]any{
			"l2": map[string]any{
				"l3": map[string]any{
					"l4": map[string]any{
						"l5": map[string]any{
							"l6": 42.0,
						},
					},
				},
			},
		},
	}
	got := extractPath(data, "l1.l2.l3.l4.l5.l6")
	assert.EqualValues(t, 42.0, got)

}

func TestExtractPath_MissingIntermediateKey_Level2(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"a": map[string]any{
			"exists": "yes",
		},
	}
	got := extractPath(data, "a.missing.c.d")
	assert.Nil(t, got)

}

func TestExtractPath_IntermediateIsArray(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"a": map[string]any{
			"b": []any{1, 2, 3},
		},
	}
	got := extractPath(data, "a.b.c")
	assert.Nil(t, got)

}

func TestExtractPath_IntermediateIsString(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"a": map[string]any{
			"b": "just a string",
		},
	}
	got := extractPath(data, "a.b.c")
	assert.Nil(t, got)

}

func TestExtractPath_IntermediateIsNumber(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"a": 42,
	}
	got := extractPath(data, "a.b")
	assert.Nil(t, got)

}

func TestExtractPath_IntermediateIsNil(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"a": map[string]any{
			"b": nil,
		},
	}
	got := extractPath(data, "a.b.c")
	assert.Nil(t, got)

}

func TestExtractPath_DeepValueIsMap(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"nested_key": "nested_val",
				},
			},
		},
	}
	got := extractPath(data, "a.b.c")
	m, ok := got.(map[string]any)
	require.True(t,
		ok)
	assert.Equal(t,
		"nested_val", m["nested_key"])

}

// MaybeTriggerOnFailure edge cases not covered by job_chaining_test.go.

func TestMaybeTriggerOnFailure_NilRunAndJob(t *testing.T) {
	t.Parallel()
	oct := NewOnCompleteTrigger(nil, nil, nil, nil, nil)
	// Should not panic.
	oct.MaybeTriggerOnFailure(context.Background(), nil, nil, "err")
	oct.MaybeTriggerOnFailure(context.Background(), &domain.JobRun{}, nil, "err")
	oct.MaybeTriggerOnFailure(context.Background(), nil, &domain.Job{}, "err")
}

func TestMaybeTriggerOnFailure_NoTriggerConfigured(t *testing.T) {
	t.Parallel()
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, nil, enqueuer, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusDeadLetter}
	job := &domain.Job{
		ID:        "job-1",
		ProjectID: "proj-1",
		// Neither OnFailureTriggerWorkflow nor OnFailureTriggerJob set.
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "boom")

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 0)

}

func TestMaybeTriggerOnFailure_TriggersJobAndWorkflow(t *testing.T) {
	t.Parallel()

	wfLookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/error-flow": {ID: "wf-err-1"},
		},
	}
	wfTrigger := &mockWorkflowTriggerer{}
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/error-handler": {ID: "job-err-1"},
		},
	}
	enqueuer := &mockJobEnqueuer{}

	oct := NewOnCompleteTrigger(wfLookup, wfTrigger, jobLookup, enqueuer, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCrashed}
	job := &domain.Job{
		ID:                       "job-1",
		ProjectID:                "proj-1",
		OnFailureTriggerWorkflow: "error-flow",
		OnFailureTriggerJob:      "error-handler",
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "oom killed")

	wfTrigger.mu.Lock()
	assert.Len(t, wfTrigger.
		calls, 1)

	wfTrigger.mu.Unlock()

	enqueuer.mu.Lock()
	assert.Len(t, enqueuer.
		calls, 1)

	enqueuer.mu.Unlock()
}

func TestMaybeTriggerOnFailure_WorkflowLookupError_ContinuesToJob(t *testing.T) {
	t.Parallel()

	wfLookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{}, // empty => lookup returns error
	}
	wfTrigger := &mockWorkflowTriggerer{}
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/handler": {ID: "job-h"},
		},
	}
	enqueuer := &mockJobEnqueuer{}

	oct := NewOnCompleteTrigger(wfLookup, wfTrigger, jobLookup, enqueuer, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusSystemFailed}
	job := &domain.Job{
		ID:                       "job-1",
		ProjectID:                "proj-1",
		OnFailureTriggerWorkflow: "missing-flow",
		OnFailureTriggerJob:      "handler",
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "system error")

	// Workflow trigger should have been attempted but failed silently.
	wfTrigger.mu.Lock()
	assert.Len(t, wfTrigger.
		calls, 0)

	wfTrigger.mu.Unlock()

	// Job trigger should still succeed.
	enqueuer.mu.Lock()
	assert.Len(t, enqueuer.
		calls, 1)

	enqueuer.mu.Unlock()
}

func TestMaybeTriggerOnFailure_ChainDepthLimit(t *testing.T) {
	t.Parallel()

	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/handler": {ID: "job-h"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{
		ID:           "run-deep",
		Status:       domain.StatusDeadLetter,
		LineageDepth: domain.MaxJobChainDepth, // already at max
	}
	job := &domain.Job{
		ID:                  "job-1",
		ProjectID:           "proj-1",
		OnFailureTriggerJob: "handler",
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "error")

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 0)

}

func TestMaybeTriggerOnFailure_JobEnqueueError_NoPanic(t *testing.T) {
	t.Parallel()

	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/handler": {ID: "job-h"},
		},
	}
	enqueuer := &mockJobEnqueuer{err: fmt.Errorf("queue full")}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCrashed}
	job := &domain.Job{
		ID:                  "job-1",
		ProjectID:           "proj-1",
		OnFailureTriggerJob: "handler",
	}

	// Should not panic even when enqueue fails.
	oct.MaybeTriggerOnFailure(context.Background(), run, job, "error")
}

func TestMaybeTriggerOnFailure_FailurePayloadMapping(t *testing.T) {
	t.Parallel()

	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/handler": {ID: "job-h"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{
		ID:         "run-1",
		Status:     domain.StatusSystemFailed,
		ErrorClass: "timeout",
		Attempt:    5,
	}
	job := &domain.Job{
		ID:                      "job-1",
		ProjectID:               "proj-1",
		OnFailureTriggerJob:     "handler",
		OnFailurePayloadMapping: json.RawMessage(`{"err":"error","cls":"error_class"}`),
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "request timeout")

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)

	var payload map[string]any
	require.NoError(
		t, json.Unmarshal(enqueuer.
			calls[0].
			run.Payload, &payload))
	assert.Equal(t,
		"request timeout",
		payload["err"])
	assert.Equal(t,
		"timeout", payload["cls"])

	// Mapped payload should only have the mapped keys.
	if _, has := payload["source_run_id"]; has {
		assert.Fail(t,

			"mapped payload should not include unmapped keys")
	}
}

// MaybeTrigger (on_complete) edge cases.

func TestMaybeTrigger_BothWorkflowAndJob(t *testing.T) {
	t.Parallel()

	wfLookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-deploy-1"},
		},
	}
	wfTrigger := &mockWorkflowTriggerer{}
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/process": {ID: "job-proc-1"},
		},
	}
	enqueuer := &mockJobEnqueuer{}

	oct := NewOnCompleteTrigger(wfLookup, wfTrigger, jobLookup, enqueuer, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
		OnCompleteTriggerJob:      "process",
	}
	result := json.RawMessage(`{"output":"done"}`)

	oct.MaybeTrigger(context.Background(), run, job, result)

	wfTrigger.mu.Lock()
	assert.Len(t, wfTrigger.
		calls, 1)

	wfTrigger.mu.Unlock()

	enqueuer.mu.Lock()
	assert.Len(t, enqueuer.
		calls, 1)

	enqueuer.mu.Unlock()
}

func TestMaybeTrigger_NilInterfaces_NoPanic(t *testing.T) {
	t.Parallel()

	// All interfaces nil but triggers configured -- should not panic.
	oct := NewOnCompleteTrigger(nil, nil, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
		OnCompleteTriggerJob:      "process",
	}

	oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))
}

func TestMaybeTrigger_ConcurrentCalls(t *testing.T) {
	t.Parallel()

	wfLookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-1"},
		},
	}
	wfTrigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(wfLookup, wfTrigger, nil, nil, nil)

	var wg conc.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			run := &domain.JobRun{
				ID:     fmt.Sprintf("run-%d", i),
				Status: domain.StatusCompleted,
			}
			job := &domain.Job{
				ID:                        fmt.Sprintf("job-%d", i),
				ProjectID:                 "proj-1",
				OnCompleteTriggerWorkflow: "deploy",
			}
			oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))
		})
	}
	wg.Wait()

	wfTrigger.mu.Lock()
	defer wfTrigger.mu.Unlock()
	assert.Len(t, wfTrigger.
		calls, 20,
	)

}
