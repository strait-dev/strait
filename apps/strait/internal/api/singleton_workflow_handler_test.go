package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"strait/internal/domain"
)

// TestHandleTriggerWorkflow_Singleton exercises the singleton response mapping in
// handleTriggerWorkflow: every outcome the engine can report must surface the
// additive singleton_outcome / singleton_holder_run_id fields without changing the
// normal WorkflowRun shape, and a drop must return outcome-only with no run id.
func TestHandleTriggerWorkflow_Singleton(t *testing.T) {
	t.Parallel()

	newSrv := func(t *testing.T, trigger *mockWorkflowTrigger) *Server {
		t.Helper()
		ms := &APIStoreMock{
			GetWorkflowFunc: func(_ context.Context, id string) (*domain.Workflow, error) {
				return &domain.Workflow{ID: id, Enabled: true}, nil
			},
		}
		return newWorkflowTestServer(t, ms, &mockQueue{}, &mockPublisher{}, trigger)
	}

	decode := func(t *testing.T, body []byte) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("decode response: %v (%s)", err, string(body))
		}
		return m
	}

	t.Run("dispatched returns run with outcome", func(t *testing.T) {
		t.Parallel()
		trigger := &mockWorkflowTrigger{
			triggerWorkflowWithOutcomeFn: func(_ context.Context, workflowID, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, domain.SingletonOutcome, string, error) {
				return &domain.WorkflowRun{ID: "wr-1", WorkflowID: workflowID, Status: domain.WfStatusRunning, SingletonKey: "acct-1"}, domain.SingletonOutcomeDispatched, "", nil
			},
		}
		srv := newSrv(t, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{"payload":{"id":"acct-1"}}`))

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		m := decode(t, w.Body.Bytes())
		if m["id"] != "wr-1" {
			t.Fatalf("id = %v, want wr-1", m["id"])
		}
		if m["singleton_outcome"] != string(domain.SingletonOutcomeDispatched) {
			t.Fatalf("singleton_outcome = %v, want dispatched", m["singleton_outcome"])
		}
		if _, ok := m["singleton_holder_run_id"]; ok {
			t.Fatalf("dispatched must omit singleton_holder_run_id, got %v", m["singleton_holder_run_id"])
		}
	})

	t.Run("dropped returns outcome only with holder", func(t *testing.T) {
		t.Parallel()
		trigger := &mockWorkflowTrigger{
			triggerWorkflowWithOutcomeFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, domain.SingletonOutcome, string, error) {
				return nil, domain.SingletonOutcomeDropped, "wr-holder", nil
			},
		}
		srv := newSrv(t, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{"payload":{"id":"acct-1"}}`))

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		m := decode(t, w.Body.Bytes())
		if m["singleton_outcome"] != string(domain.SingletonOutcomeDropped) {
			t.Fatalf("singleton_outcome = %v, want dropped", m["singleton_outcome"])
		}
		if m["singleton_holder_run_id"] != "wr-holder" {
			t.Fatalf("singleton_holder_run_id = %v, want wr-holder", m["singleton_holder_run_id"])
		}
		if _, ok := m["id"]; ok {
			t.Fatalf("dropped must not return a run id, got %v", m["id"])
		}
	})

	t.Run("queued_behind returns parked run with outcome", func(t *testing.T) {
		t.Parallel()
		trigger := &mockWorkflowTrigger{
			triggerWorkflowWithOutcomeFn: func(_ context.Context, workflowID, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, domain.SingletonOutcome, string, error) {
				return &domain.WorkflowRun{ID: "wr-2", WorkflowID: workflowID, Status: domain.WfStatusQueued, SingletonKey: "acct-1"}, domain.SingletonOutcomeQueuedBehind, "wr-holder", nil
			},
		}
		srv := newSrv(t, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{"payload":{"id":"acct-1"}}`))

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		m := decode(t, w.Body.Bytes())
		if m["id"] != "wr-2" {
			t.Fatalf("id = %v, want wr-2", m["id"])
		}
		if m["status"] != string(domain.WfStatusQueued) {
			t.Fatalf("status = %v, want queued", m["status"])
		}
		if m["singleton_outcome"] != string(domain.SingletonOutcomeQueuedBehind) {
			t.Fatalf("singleton_outcome = %v, want queued_behind", m["singleton_outcome"])
		}
		if m["singleton_holder_run_id"] != "wr-holder" {
			t.Fatalf("singleton_holder_run_id = %v, want wr-holder", m["singleton_holder_run_id"])
		}
	})

	t.Run("replaced returns newcomer with previous holder", func(t *testing.T) {
		t.Parallel()
		trigger := &mockWorkflowTrigger{
			triggerWorkflowWithOutcomeFn: func(_ context.Context, workflowID, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, domain.SingletonOutcome, string, error) {
				return &domain.WorkflowRun{ID: "wr-3", WorkflowID: workflowID, Status: domain.WfStatusRunning, SingletonKey: "acct-1"}, domain.SingletonOutcomeReplaced, "wr-old", nil
			},
		}
		srv := newSrv(t, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{"payload":{"id":"acct-1"}}`))

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		m := decode(t, w.Body.Bytes())
		if m["id"] != "wr-3" {
			t.Fatalf("id = %v, want wr-3", m["id"])
		}
		if m["singleton_outcome"] != string(domain.SingletonOutcomeReplaced) {
			t.Fatalf("singleton_outcome = %v, want replaced", m["singleton_outcome"])
		}
		if m["singleton_holder_run_id"] != "wr-old" {
			t.Fatalf("singleton_holder_run_id = %v, want wr-old", m["singleton_holder_run_id"])
		}
	})

	t.Run("unresolvable key returns 400", func(t *testing.T) {
		t.Parallel()
		trigger := &mockWorkflowTrigger{
			triggerWorkflowWithOutcomeFn: func(_ context.Context, _, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, domain.SingletonOutcome, string, error) {
				return nil, "", "", domain.ErrSingletonKeyUnresolvable
			},
		}
		srv := newSrv(t, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{"payload":{}}`))

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("no singleton configured returns plain run", func(t *testing.T) {
		t.Parallel()
		trigger := &mockWorkflowTrigger{
			triggerWorkflowWithOutcomeFn: func(_ context.Context, workflowID, _ string, _ json.RawMessage, _ string, _ []domain.StepOverride, _ map[string]string) (*domain.WorkflowRun, domain.SingletonOutcome, string, error) {
				return &domain.WorkflowRun{ID: "wr-4", WorkflowID: workflowID, Status: domain.WfStatusRunning}, "", "", nil
			},
		}
		srv := newSrv(t, trigger)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/workflows/wf-1/trigger", `{}`))

		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d, want 201: %s", w.Code, w.Body.String())
		}
		m := decode(t, w.Body.Bytes())
		if m["id"] != "wr-4" {
			t.Fatalf("id = %v, want wr-4", m["id"])
		}
		if _, ok := m["singleton_outcome"]; ok {
			t.Fatalf("no singleton config must omit singleton_outcome, got %v", m["singleton_outcome"])
		}
	})
}
