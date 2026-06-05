package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock helpers for job chaining.

type mockJobLookup struct {
	mu   sync.Mutex
	jobs map[string]*domain.Job
}

func (m *mockJobLookup) GetJobBySlug(_ context.Context, projectID, slug string) (*domain.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := projectID + "/" + slug
	if j, ok := m.jobs[key]; ok {
		return j, nil
	}
	return nil, fmt.Errorf("job %q not found in project %q", slug, projectID)
}

type enqueueCall struct {
	run *domain.JobRun
}

type mockJobEnqueuer struct {
	mu    sync.Mutex
	calls []enqueueCall
	err   error
}

func (m *mockJobEnqueuer) Enqueue(_ context.Context, run *domain.JobRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, enqueueCall{run: run})
	return m.err
}

// Unit tests for on_complete job triggers.

func TestOnComplete_TriggersDownstreamJob(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/process": {ID: "job-process-1"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                   "job-1",
		ProjectID:            "proj-1",
		OnCompleteTriggerJob: "process",
	}
	result := json.RawMessage(`{"output":"done"}`)

	oct.MaybeTrigger(context.Background(), run, job, result)

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)

	call := enqueuer.calls[0]
	assert.Equal(t,
		"job-process-1",
		call.run.
			JobID)
	assert.Equal(t,
		domain.TriggerJobChain,

		call.run.TriggeredBy,
	)
	assert.Equal(t,
		"run-1", call.run.
			ParentRunID,
	)
	assert.Equal(t,
		string(result), string(call.
			run.Payload,
		))

}

func TestOnComplete_TriggersDownstreamWorkflow(t *testing.T) {
	t.Parallel()
	lookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-deploy-1"},
		},
	}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
	}
	result := json.RawMessage(`{"output":"done"}`)

	oct.MaybeTrigger(context.Background(), run, job, result)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	require.Len(t, trigger.
		calls, 1)

}

func TestOnComplete_TriggersBothJobAndWorkflow(t *testing.T) {
	t.Parallel()
	wfLookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-1"},
		},
	}
	wfTrigger := &mockWorkflowTriggerer{}
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/process": {ID: "job-2"},
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

	oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

	wfTrigger.mu.Lock()
	assert.Len(t, wfTrigger.
		calls, 1)

	wfTrigger.mu.Unlock()

	enqueuer.mu.Lock()
	assert.Len(t, enqueuer.
		calls, 1)

	enqueuer.mu.Unlock()
}

func TestOnComplete_PassesOutputAsPayload(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/next": {ID: "job-next"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	result := json.RawMessage(`{"user":{"id":"u-123"},"count":42}`)
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                   "job-1",
		ProjectID:            "proj-1",
		OnCompleteTriggerJob: "next",
	}

	oct.MaybeTrigger(context.Background(), run, job, result)

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)
	assert.Equal(t,
		string(result), string(enqueuer.
			calls[0].run.
			Payload))

}

func TestOnComplete_PayloadMappingForJob(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/next": {ID: "job-next"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	result := json.RawMessage(`{"user":{"id":"u-123","name":"Alice"},"extra":"ignored"}`)
	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                       "job-1",
		ProjectID:                "proj-1",
		OnCompleteTriggerJob:     "next",
		OnCompletePayloadMapping: json.RawMessage(`{"user_id":"user.id","name":"user.name"}`),
	}

	oct.MaybeTrigger(context.Background(), run, job, result)

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)

	var mapped map[string]any
	require.NoError(
		t, json.Unmarshal(enqueuer.
			calls[0].
			run.Payload,
			&mapped,
		))
	assert.Equal(t,
		"u-123", mapped["user_id"])
	assert.Equal(t,
		"Alice", mapped["name"])

	if _, ok := mapped["extra"]; ok {
		assert.Fail(t,

			"extra should not be present after mapping")
	}
}

func TestOnComplete_NoTriggerConfigured(t *testing.T) {
	t.Parallel()
	enqueuer := &mockJobEnqueuer{}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(nil, trigger, nil, enqueuer, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{ID: "job-1", ProjectID: "proj-1"}

	oct.MaybeTrigger(context.Background(), run, job, nil)

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 0)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	require.Len(t, trigger.
		calls, 0)

}

func TestOnComplete_JobNotFound(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{jobs: map[string]*domain.Job{}}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                   "job-1",
		ProjectID:            "proj-1",
		OnCompleteTriggerJob: "nonexistent",
	}

	oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 0)

}

// Unit tests for on_failure triggers.

func TestOnFailure_TriggersOnDeadLetter(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/error-handler": {ID: "job-error-1"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{
		ID:         "run-1",
		Status:     domain.StatusDeadLetter,
		ErrorClass: "server",
		Attempt:    3,
		Payload:    json.RawMessage(`{"order_id":"123"}`),
	}
	job := &domain.Job{
		ID:                  "job-1",
		ProjectID:           "proj-1",
		OnFailureTriggerJob: "error-handler",
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "connection refused")

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)

	call := enqueuer.calls[0]
	assert.Equal(t,
		domain.TriggerJobFailure,

		call.run.
			TriggeredBy,
	)

	var payload map[string]any
	require.NoError(
		t, json.Unmarshal(call.run.
			Payload,
			&payload,
		))
	assert.Equal(t,
		"connection refused",
		payload["error"])
	assert.Equal(t,
		"job-1", payload["source_job_id"])

}

func TestOnFailure_TriggersOnTimedOut(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/timeout-handler": {ID: "job-to-1"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusTimedOut}
	job := &domain.Job{
		ID:                  "job-1",
		ProjectID:           "proj-1",
		OnFailureTriggerJob: "timeout-handler",
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "execution timed out")

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)

}

func TestOnFailure_DoesNotTriggerOnSuccess(t *testing.T) {
	t.Parallel()
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, nil, enqueuer, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                  "job-1",
		ProjectID:           "proj-1",
		OnFailureTriggerJob: "error-handler",
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "")

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 0)

}

func TestOnFailure_TriggersWorkflow(t *testing.T) {
	t.Parallel()
	wfLookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/error-flow": {ID: "wf-err-1"},
		},
	}
	wfTrigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(wfLookup, wfTrigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusDeadLetter}
	job := &domain.Job{
		ID:                       "job-1",
		ProjectID:                "proj-1",
		OnFailureTriggerWorkflow: "error-flow",
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "database error")

	wfTrigger.mu.Lock()
	defer wfTrigger.mu.Unlock()
	require.Len(t, wfTrigger.
		calls, 1,
	)
	assert.Equal(t,
		domain.TriggerJobFailure,

		wfTrigger.
			calls[0].triggeredBy,
	)

}

func TestOnFailure_PassesErrorContext(t *testing.T) {
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
		Status:     domain.StatusCrashed,
		ErrorClass: "server",
		Attempt:    2,
		Payload:    json.RawMessage(`{"input":"data"}`),
	}
	job := &domain.Job{
		ID:                  "job-1",
		ProjectID:           "proj-1",
		OnFailureTriggerJob: "handler",
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "segfault")

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)

	var payload map[string]any
	require.NoError(
		t, json.Unmarshal(enqueuer.
			calls[0].
			run.Payload,
			&payload,
		))
	assert.Equal(t,
		"segfault", payload["error"])
	assert.Equal(t,
		"server", payload["error_class"])
	assert.Equal(t,
		"crashed", payload["status"])
	assert.Equal(t,
		float64(2), payload["attempt"])

}

func TestOnFailure_PayloadMapping(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/handler": {ID: "job-h"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{
		ID:     "run-1",
		Status: domain.StatusDeadLetter,
	}
	job := &domain.Job{
		ID:                      "job-1",
		ProjectID:               "proj-1",
		OnFailureTriggerJob:     "handler",
		OnFailurePayloadMapping: json.RawMessage(`{"err":"error","run":"source_run_id"}`),
	}

	oct.MaybeTriggerOnFailure(context.Background(), run, job, "boom")

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)

	var payload map[string]any
	require.NoError(
		t, json.Unmarshal(enqueuer.
			calls[0].
			run.Payload,
			&payload,
		))
	assert.Equal(t,
		"boom", payload["err"])
	assert.Equal(t,
		"run-1", payload["run"])

}

// Chain depth enforcement tests.

func TestChainDepth_Enforcement(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/next": {ID: "job-next"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{
		ID:           "run-1",
		Status:       domain.StatusCompleted,
		LineageDepth: domain.MaxJobChainDepth, // at limit
	}
	job := &domain.Job{
		ID:                   "job-1",
		ProjectID:            "proj-1",
		OnCompleteTriggerJob: "next",
	}

	oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 0)

}

func TestChainDepth_Propagation(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/next": {ID: "job-next"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{
		ID:           "run-1",
		Status:       domain.StatusCompleted,
		LineageDepth: 5,
	}
	job := &domain.Job{
		ID:                   "job-1",
		ProjectID:            "proj-1",
		OnCompleteTriggerJob: "next",
	}

	oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)
	assert.EqualValues(t, 6, enqueuer.calls[0].run.
		LineageDepth,
	)

}

func TestChainDepth_StartsAtZero(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/next": {ID: "job-next"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{
		ID:           "run-1",
		Status:       domain.StatusCompleted,
		LineageDepth: 0,
	}
	job := &domain.Job{
		ID:                   "job-1",
		ProjectID:            "proj-1",
		OnCompleteTriggerJob: "next",
	}

	oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)
	assert.EqualValues(t, 1, enqueuer.calls[0].run.
		LineageDepth,
	)

}

func TestChainDepth_ExactlyAtLimit(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"proj-1/next": {ID: "job-next"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	// depth = 9 should work (limit is 10)
	run := &domain.JobRun{
		ID:           "run-ok",
		Status:       domain.StatusCompleted,
		LineageDepth: domain.MaxJobChainDepth - 1,
	}
	job := &domain.Job{
		ID:                   "job-1",
		ProjectID:            "proj-1",
		OnCompleteTriggerJob: "next",
	}

	oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

	enqueuer.mu.Lock()
	okCalls := len(enqueuer.calls)
	enqueuer.mu.Unlock()
	require.EqualValues(t, 1, okCalls)

	// depth = 10 should NOT work
	run2 := &domain.JobRun{
		ID:           "run-blocked",
		Status:       domain.StatusCompleted,
		LineageDepth: domain.MaxJobChainDepth,
	}

	oct.MaybeTrigger(context.Background(), run2, job, json.RawMessage(`{}`))

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)

}

// Fuzz tests for job chaining.

func FuzzPayloadMapping(f *testing.F) {
	f.Add([]byte(`{"a":"b"}`), []byte(`{"out":"a"}`))
	f.Add([]byte(`{"nested":{"deep":{"val":42}}}`), []byte(`{"v":"nested.deep.val"}`))
	f.Add([]byte(`[1,2,3]`), []byte(`{"x":"0"}`))
	f.Add([]byte(`"just a string"`), []byte(`{"x":"key"}`))
	f.Add([]byte(`null`), []byte(`{"x":"key"}`))
	f.Add([]byte(``), []byte(`{"x":"key"}`))
	f.Add([]byte(`{"a":"b"}`), []byte(``))

	f.Fuzz(func(t *testing.T, result, mapping []byte) {
		// Must never panic.
		_, _ = applyPayloadMapping(json.RawMessage(result), json.RawMessage(mapping))
	})
}

func FuzzChainDepthOverflow(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(9)
	f.Add(10)
	f.Add(11)
	f.Add(100)
	f.Add(-1)
	f.Add(1<<31 - 1)

	f.Fuzz(func(t *testing.T, depth int) {
		jobLookup := &mockJobLookup{
			jobs: map[string]*domain.Job{
				"p/j": {ID: "target"},
			},
		}
		enqueuer := &mockJobEnqueuer{}
		oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

		run := &domain.JobRun{
			ID:           "r",
			Status:       domain.StatusCompleted,
			LineageDepth: depth,
		}
		job := &domain.Job{
			ID:                   "j",
			ProjectID:            "p",
			OnCompleteTriggerJob: "j",
		}

		// Must never panic.
		oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

		enqueuer.mu.Lock()
		defer enqueuer.mu.Unlock()

		if depth >= domain.MaxJobChainDepth {
			assert.Len(t, enqueuer.
				calls, 0)

		} else if depth >= 0 {
			if len(enqueuer.calls) != 1 {
				assert.Failf(t, "test failure",

					"depth %d < max %d should trigger", depth, domain.MaxJobChainDepth)
			} else if enqueuer.calls[0].run.LineageDepth != depth+1 {
				assert.Failf(t, "test failure",

					"downstream depth = %d, want %d", enqueuer.calls[0].run.LineageDepth, depth+1)
			}
		}
	})
}

// Adversarial tests for job chaining.

func TestChaining_MalformedPayloadMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		mapping string
	}{
		{"invalid json", `not valid json`},
		{"empty object", `{}`},
		{"null", `null`},
		{"array", `["a","b"]`},
		{"nested mapping", `{"a":{"b":"c"}}`},
		{"empty string value", `{"a":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := json.RawMessage(`{"key":"value"}`)
			_, _ = applyPayloadMapping(result, json.RawMessage(tt.mapping))
			// Must not panic.
		})
	}
}

func TestChaining_SQLInjectionInJobSlug(t *testing.T) {
	t.Parallel()
	maliciousSlugs := []string{
		"'; DROP TABLE jobs; --",
		"job' OR '1'='1",
		"job\x00null",
		"../../../etc/passwd",
		strings.Repeat("a", 10000),
	}

	for _, slug := range maliciousSlugs {
		t.Run(slug[:min(len(slug), 30)], func(t *testing.T) {
			t.Parallel()
			jobLookup := &mockJobLookup{jobs: map[string]*domain.Job{}}
			enqueuer := &mockJobEnqueuer{}
			oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

			run := &domain.JobRun{ID: "r", Status: domain.StatusCompleted}
			job := &domain.Job{
				ID:                   "j",
				ProjectID:            "p",
				OnCompleteTriggerJob: slug,
			}

			// Must not panic.
			oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

			enqueuer.mu.Lock()
			defer enqueuer.mu.Unlock()
			assert.Len(t, enqueuer.
				calls, 0)

		})
	}
}

func TestChaining_5MBOutputPayload(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"p/next": {ID: "job-next"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	// Build ~5MB payload.
	largeStr := strings.Repeat("x", 5*1024*1024)
	result, _ := json.Marshal(map[string]string{"data": largeStr})

	run := &domain.JobRun{ID: "r", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                   "j",
		ProjectID:            "p",
		OnCompleteTriggerJob: "next",
	}

	oct.MaybeTrigger(context.Background(), run, job, result)

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)
	assert.GreaterOrEqual(t, len(enqueuer.
		calls[0].run.
		Payload),
		5*1024*
			1024)

}

func TestChaining_ConcurrentCompletions(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"p/next": {ID: "job-next"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	var wg conc.WaitGroup
	for i := range 50 {
		wg.Go(func() {
			run := &domain.JobRun{
				ID:     fmt.Sprintf("run-%d", i),
				Status: domain.StatusCompleted,
			}
			job := &domain.Job{
				ID:                   fmt.Sprintf("job-%d", i),
				ProjectID:            "p",
				OnCompleteTriggerJob: "next",
			}
			oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))
		})
	}
	wg.Wait()

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	assert.Len(t, enqueuer.
		calls, 50)

}

func TestChaining_NilOutputWithMapping(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"p/next": {ID: "job-next"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	run := &domain.JobRun{ID: "r", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                       "j",
		ProjectID:                "p",
		OnCompleteTriggerJob:     "next",
		OnCompletePayloadMapping: json.RawMessage(`{"key":"some.path"}`),
	}

	// nil result with non-nil mapping should not panic.
	oct.MaybeTrigger(context.Background(), run, job, nil)

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)

}

func TestChaining_UnicodeInPayload(t *testing.T) {
	t.Parallel()
	jobLookup := &mockJobLookup{
		jobs: map[string]*domain.Job{
			"p/next": {ID: "job-next"},
		},
	}
	enqueuer := &mockJobEnqueuer{}
	oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

	result := json.RawMessage(`{"name":"Andre\u0301","city":"Sao Paulo"}`)
	run := &domain.JobRun{ID: "r", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                   "j",
		ProjectID:            "p",
		OnCompleteTriggerJob: "next",
	}

	oct.MaybeTrigger(context.Background(), run, job, result)

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	require.Len(t, enqueuer.
		calls, 1)

}

func TestOnFailure_AllTerminalFailureStatuses(t *testing.T) {
	t.Parallel()
	terminalFailures := []domain.RunStatus{
		domain.StatusDeadLetter,
		domain.StatusTimedOut,
		domain.StatusCrashed,
		domain.StatusSystemFailed,
	}

	for _, status := range terminalFailures {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			jobLookup := &mockJobLookup{
				jobs: map[string]*domain.Job{
					"p/handler": {ID: "job-h"},
				},
			}
			enqueuer := &mockJobEnqueuer{}
			oct := NewOnCompleteTrigger(nil, nil, jobLookup, enqueuer, nil)

			run := &domain.JobRun{ID: "r", Status: status}
			job := &domain.Job{
				ID:                  "j",
				ProjectID:           "p",
				OnFailureTriggerJob: "handler",
			}

			oct.MaybeTriggerOnFailure(context.Background(), run, job, "err")

			enqueuer.mu.Lock()
			defer enqueuer.mu.Unlock()
			assert.Len(t, enqueuer.
				calls, 1)

		})
	}
}

func TestOnFailure_NonTerminalStatusesDoNotTrigger(t *testing.T) {
	t.Parallel()
	nonTerminalStatuses := []domain.RunStatus{
		domain.StatusQueued,
		domain.StatusDequeued,
		domain.StatusExecuting,
		domain.StatusCompleted,
		domain.StatusCanceled,
		domain.StatusExpired,
		domain.StatusWaiting,
		domain.StatusPaused,
	}

	for _, status := range nonTerminalStatuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			enqueuer := &mockJobEnqueuer{}
			oct := NewOnCompleteTrigger(nil, nil, nil, enqueuer, nil)

			run := &domain.JobRun{ID: "r", Status: status}
			job := &domain.Job{
				ID:                  "j",
				ProjectID:           "p",
				OnFailureTriggerJob: "handler",
			}

			oct.MaybeTriggerOnFailure(context.Background(), run, job, "err")

			enqueuer.mu.Lock()
			defer enqueuer.mu.Unlock()
			assert.Len(t, enqueuer.
				calls, 0)

		})
	}
}

func TestIsTerminalFailureStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		status domain.RunStatus
		want   bool
	}{
		{domain.StatusDeadLetter, true},
		{domain.StatusTimedOut, true},
		{domain.StatusCrashed, true},
		{domain.StatusSystemFailed, true},
		{domain.StatusCompleted, false},
		{domain.StatusQueued, false},
		{domain.StatusCanceled, false},
		{domain.StatusExpired, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			t.Parallel()
			got := isTerminalFailureStatus(tt.status)
			assert.Equal(t,
				tt.want, got)

		})
	}
}
