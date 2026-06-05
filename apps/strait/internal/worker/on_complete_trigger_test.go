package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockWorkflowLookup struct {
	mu        sync.Mutex
	workflows map[string]*domain.Workflow
}

func (m *mockWorkflowLookup) GetWorkflowBySlug(_ context.Context, projectID, slug string) (*domain.Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := projectID + "/" + slug
	if wf, ok := m.workflows[key]; ok {
		return wf, nil
	}
	return nil, fmt.Errorf("workflow %q not found in project %q", slug, projectID)
}

type triggerCall struct {
	workflowID  string
	projectID   string
	payload     json.RawMessage
	triggeredBy string
	extraTags   map[string]string
}

type mockWorkflowTriggerer struct {
	mu    sync.Mutex
	calls []triggerCall
	err   error
}

func (m *mockWorkflowTriggerer) TriggerWorkflow(_ context.Context, workflowID, projectID string, payload json.RawMessage, triggeredBy string, _ []domain.StepOverride, extraTags map[string]string) (*domain.WorkflowRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, triggerCall{
		workflowID:  workflowID,
		projectID:   projectID,
		payload:     payload,
		triggeredBy: triggeredBy,
		extraTags:   extraTags,
	})
	if m.err != nil {
		return nil, m.err
	}
	return &domain.WorkflowRun{ID: "wfr-test-1"}, nil
}

func TestOnCompleteTrigger_HappyPath(t *testing.T) {
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

	call := trigger.calls[0]
	assert.Equal(t,
		"wf-deploy-1", call.
			workflowID,
	)
	assert.Equal(t,
		domain.TriggerJobCompletion,

		call.
			triggeredBy,
	)
	assert.Equal(t,
		"job-1", call.extraTags["source_job_id"])
	assert.Equal(t,
		"run-1", call.extraTags["source_run_id"])

}

func TestOnCompleteTrigger_NoWorkflowConfigured(t *testing.T) {
	t.Parallel()
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(nil, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{ID: "job-1", ProjectID: "proj-1"}

	oct.MaybeTrigger(context.Background(), run, job, nil)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	require.Len(t, trigger.
		calls, 0)

}

func TestOnCompleteTrigger_NonCompletedStatus(t *testing.T) {
	t.Parallel()
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(&mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-1"},
		},
	}, trigger, nil, nil, nil)

	// Failed runs should NOT trigger.
	for _, status := range []domain.RunStatus{
		domain.StatusFailed,
		domain.StatusTimedOut,
		domain.StatusCanceled,
		domain.StatusDeadLetter,
	} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			run := &domain.JobRun{ID: "run-1", Status: status}
			job := &domain.Job{
				ID:                        "job-1",
				ProjectID:                 "proj-1",
				OnCompleteTriggerWorkflow: "deploy",
			}

			oct.MaybeTrigger(context.Background(), run, job, nil)
		})
	}

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	require.Len(t, trigger.
		calls, 0)

}

func TestOnCompleteTrigger_WorkflowNotFound(t *testing.T) {
	t.Parallel()
	lookup := &mockWorkflowLookup{workflows: map[string]*domain.Workflow{}}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "nonexistent",
	}

	// Should not panic, should not trigger.
	oct.MaybeTrigger(context.Background(), run, job, nil)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	require.Len(t, trigger.
		calls, 0)

}

func TestOnCompleteTrigger_TriggerError(t *testing.T) {
	t.Parallel()
	lookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-1"},
		},
	}
	trigger := &mockWorkflowTriggerer{err: fmt.Errorf("trigger failed")}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
	}

	// Should not panic even if trigger fails.
	oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	require.Len(t, trigger.
		calls, 1)

}

func TestOnCompleteTrigger_PayloadMapping(t *testing.T) {
	t.Parallel()
	lookup := &mockWorkflowLookup{
		workflows: map[string]*domain.Workflow{
			"proj-1/deploy": {ID: "wf-1"},
		},
	}
	trigger := &mockWorkflowTriggerer{}
	oct := NewOnCompleteTrigger(lookup, trigger, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
		OnCompletePayloadMapping:  json.RawMessage(`{"user_id":"user.id","name":"user.name"}`),
	}
	result := json.RawMessage(`{"user":{"id":"u-123","name":"Alice"},"extra":"ignored"}`)

	oct.MaybeTrigger(context.Background(), run, job, result)

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	require.Len(t, trigger.
		calls, 1)

	var mapped map[string]any
	require.NoError(
		t, json.Unmarshal(trigger.
			calls[0].payload,

			&mapped))
	assert.Equal(t,
		"u-123", mapped["user_id"])
	assert.Equal(t,
		"Alice", mapped["name"])

	if _, ok := mapped["extra"]; ok {
		assert.Fail(t,

			"mapped payload should not contain 'extra'")
	}
}

func TestExtractPath(t *testing.T) {
	t.Parallel()
	data := map[string]any{
		"user": map[string]any{
			"id":   "u-123",
			"name": "Alice",
			"address": map[string]any{
				"city": "NYC",
			},
		},
		"status": "active",
	}

	tests := []struct {
		path     string
		expected any
	}{
		{"status", "active"},
		{"user.id", "u-123"},
		{"user.name", "Alice"},
		{"user.address.city", "NYC"},
		{"nonexistent", nil},
		{"user.nonexistent", nil},
		{"user.address.nonexistent", nil},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			got := extractPath(data, tt.path)
			assert.Equal(t,
				tt.expected, got)

		})
	}
}

func TestApplyPayloadMapping_EmptyInputs(t *testing.T) {
	t.Parallel()

	// Empty result returns as-is.
	result, err := applyPayloadMapping(nil, json.RawMessage(`{"a":"b"}`))
	require.NoError(
		t, err)
	assert.Nil(t, result)

	// Empty mapping returns result as-is.
	input := json.RawMessage(`{"key":"val"}`)
	result, err = applyPayloadMapping(input, nil)
	require.NoError(
		t, err)
	assert.Equal(t,
		string(input), string(result))

}

func TestApplyPayloadMapping_NonObjectResult(t *testing.T) {
	t.Parallel()
	// Non-object result (array) should return as-is.
	input := json.RawMessage(`[1,2,3]`)
	result, err := applyPayloadMapping(input, json.RawMessage(`{"a":"0"}`))
	require.NoError(
		t, err)
	assert.Equal(t,
		string(input), string(result))

}

func TestOnCompleteTrigger_NilLookupAndTriggerer(t *testing.T) {
	t.Parallel()
	oct := NewOnCompleteTrigger(nil, nil, nil, nil, nil)

	run := &domain.JobRun{ID: "run-1", Status: domain.StatusCompleted}
	job := &domain.Job{
		ID:                        "job-1",
		ProjectID:                 "proj-1",
		OnCompleteTriggerWorkflow: "deploy",
	}

	// Should not panic.
	oct.MaybeTrigger(context.Background(), run, job, nil)
}
