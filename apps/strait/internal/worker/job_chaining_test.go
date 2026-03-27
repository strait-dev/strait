package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"strait/internal/domain"
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 enqueue call, got %d", len(enqueuer.calls))
	}
	call := enqueuer.calls[0]
	if call.run.JobID != "job-process-1" {
		t.Errorf("target job_id = %q, want %q", call.run.JobID, "job-process-1")
	}
	if call.run.TriggeredBy != domain.TriggerJobChain {
		t.Errorf("triggered_by = %q, want %q", call.run.TriggeredBy, domain.TriggerJobChain)
	}
	if call.run.ParentRunID != "run-1" {
		t.Errorf("parent_run_id = %q, want %q", call.run.ParentRunID, "run-1")
	}
	if string(call.run.Payload) != string(result) {
		t.Errorf("payload = %s, want %s", call.run.Payload, result)
	}
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
	if len(trigger.calls) != 1 {
		t.Fatalf("expected 1 workflow trigger call, got %d", len(trigger.calls))
	}
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
	if len(wfTrigger.calls) != 1 {
		t.Errorf("expected 1 workflow trigger, got %d", len(wfTrigger.calls))
	}
	wfTrigger.mu.Unlock()

	enqueuer.mu.Lock()
	if len(enqueuer.calls) != 1 {
		t.Errorf("expected 1 job enqueue, got %d", len(enqueuer.calls))
	}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(enqueuer.calls))
	}
	if string(enqueuer.calls[0].run.Payload) != string(result) {
		t.Errorf("payload not passed through: got %s", enqueuer.calls[0].run.Payload)
	}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(enqueuer.calls))
	}

	var mapped map[string]any
	if err := json.Unmarshal(enqueuer.calls[0].run.Payload, &mapped); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if mapped["user_id"] != "u-123" {
		t.Errorf("user_id = %v, want u-123", mapped["user_id"])
	}
	if mapped["name"] != "Alice" {
		t.Errorf("name = %v, want Alice", mapped["name"])
	}
	if _, ok := mapped["extra"]; ok {
		t.Error("extra should not be present after mapping")
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
	if len(enqueuer.calls) != 0 {
		t.Fatalf("expected 0 enqueue calls, got %d", len(enqueuer.calls))
	}

	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if len(trigger.calls) != 0 {
		t.Fatalf("expected 0 workflow trigger calls, got %d", len(trigger.calls))
	}
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
	if len(enqueuer.calls) != 0 {
		t.Fatalf("expected 0 enqueue calls for missing job, got %d", len(enqueuer.calls))
	}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 enqueue call, got %d", len(enqueuer.calls))
	}
	call := enqueuer.calls[0]
	if call.run.TriggeredBy != domain.TriggerJobFailure {
		t.Errorf("triggered_by = %q, want %q", call.run.TriggeredBy, domain.TriggerJobFailure)
	}

	var payload map[string]any
	if err := json.Unmarshal(call.run.Payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["error"] != "connection refused" {
		t.Errorf("error = %v, want 'connection refused'", payload["error"])
	}
	if payload["source_job_id"] != "job-1" {
		t.Errorf("source_job_id = %v, want job-1", payload["source_job_id"])
	}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 call for timed_out, got %d", len(enqueuer.calls))
	}
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
	if len(enqueuer.calls) != 0 {
		t.Fatalf("expected 0 calls for successful run, got %d", len(enqueuer.calls))
	}
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
	if len(wfTrigger.calls) != 1 {
		t.Fatalf("expected 1 workflow trigger, got %d", len(wfTrigger.calls))
	}
	if wfTrigger.calls[0].triggeredBy != domain.TriggerJobFailure {
		t.Errorf("triggered_by = %q, want %q", wfTrigger.calls[0].triggeredBy, domain.TriggerJobFailure)
	}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(enqueuer.calls))
	}

	var payload map[string]any
	if err := json.Unmarshal(enqueuer.calls[0].run.Payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["error"] != "segfault" {
		t.Errorf("error = %v, want segfault", payload["error"])
	}
	if payload["error_class"] != "server" {
		t.Errorf("error_class = %v, want server", payload["error_class"])
	}
	if payload["status"] != "crashed" {
		t.Errorf("status = %v, want crashed", payload["status"])
	}
	if payload["attempt"] != float64(2) {
		t.Errorf("attempt = %v, want 2", payload["attempt"])
	}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(enqueuer.calls))
	}

	var payload map[string]any
	if err := json.Unmarshal(enqueuer.calls[0].run.Payload, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["err"] != "boom" {
		t.Errorf("err = %v, want boom", payload["err"])
	}
	if payload["run"] != "run-1" {
		t.Errorf("run = %v, want run-1", payload["run"])
	}
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
	if len(enqueuer.calls) != 0 {
		t.Fatalf("expected 0 calls at max depth, got %d", len(enqueuer.calls))
	}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(enqueuer.calls))
	}
	if enqueuer.calls[0].run.LineageDepth != 6 {
		t.Errorf("lineage_depth = %d, want 6", enqueuer.calls[0].run.LineageDepth)
	}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(enqueuer.calls))
	}
	if enqueuer.calls[0].run.LineageDepth != 1 {
		t.Errorf("lineage_depth = %d, want 1", enqueuer.calls[0].run.LineageDepth)
	}
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
	if okCalls != 1 {
		t.Fatalf("depth %d should trigger, got %d calls", domain.MaxJobChainDepth-1, okCalls)
	}

	// depth = 10 should NOT work
	run2 := &domain.JobRun{
		ID:           "run-blocked",
		Status:       domain.StatusCompleted,
		LineageDepth: domain.MaxJobChainDepth,
	}

	oct.MaybeTrigger(context.Background(), run2, job, json.RawMessage(`{}`))

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	if len(enqueuer.calls) != 1 {
		t.Fatalf("depth %d should not trigger, got %d total calls", domain.MaxJobChainDepth, len(enqueuer.calls))
	}
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
			if len(enqueuer.calls) != 0 {
				t.Errorf("depth %d >= max %d should not trigger", depth, domain.MaxJobChainDepth)
			}
		} else if depth >= 0 {
			if len(enqueuer.calls) != 1 {
				t.Errorf("depth %d < max %d should trigger", depth, domain.MaxJobChainDepth)
			} else if enqueuer.calls[0].run.LineageDepth != depth+1 {
				t.Errorf("downstream depth = %d, want %d", enqueuer.calls[0].run.LineageDepth, depth+1)
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
			if len(enqueuer.calls) != 0 {
				t.Error("malicious slug should not resolve to a job")
			}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(enqueuer.calls))
	}
	if len(enqueuer.calls[0].run.Payload) < 5*1024*1024 {
		t.Error("large payload should be passed through")
	}
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

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			run := &domain.JobRun{
				ID:     fmt.Sprintf("run-%d", idx),
				Status: domain.StatusCompleted,
			}
			job := &domain.Job{
				ID:                   fmt.Sprintf("job-%d", idx),
				ProjectID:            "p",
				OnCompleteTriggerJob: "next",
			}
			oct.MaybeTrigger(context.Background(), run, job, json.RawMessage(`{}`))
		}(i)
	}
	wg.Wait()

	enqueuer.mu.Lock()
	defer enqueuer.mu.Unlock()
	if len(enqueuer.calls) != 50 {
		t.Errorf("expected 50 concurrent triggers, got %d", len(enqueuer.calls))
	}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(enqueuer.calls))
	}
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
	if len(enqueuer.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(enqueuer.calls))
	}
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
			if len(enqueuer.calls) != 1 {
				t.Errorf("status %s should trigger on_failure, got %d calls", status, len(enqueuer.calls))
			}
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
			if len(enqueuer.calls) != 0 {
				t.Errorf("status %s should NOT trigger on_failure, got %d calls", status, len(enqueuer.calls))
			}
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
			if got != tt.want {
				t.Errorf("isTerminalFailureStatus(%s) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
