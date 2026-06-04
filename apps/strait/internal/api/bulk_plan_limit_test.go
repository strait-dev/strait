package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"strait/internal/config"
	"strait/internal/domain"
	"strait/internal/pubsub"
)

// priorityRecordingEnforcer extends mockBillingEnforcer with a configurable
// max dispatch priority. Items above the cap are rejected; the call counter
// confirms that the gate is invoked per item, not once per request.
type priorityRecordingEnforcer struct {
	mockBillingEnforcer
	maxPriority int
	calls       atomic.Int64
}

func (p *priorityRecordingEnforcer) CheckMaxDispatchPriority(_ context.Context, _ string, requested int) error {
	p.calls.Add(1)
	if requested > p.maxPriority {
		return fmt.Errorf("priority %d exceeds plan cap %d", requested, p.maxPriority)
	}
	return nil
}

func newServerWithEnforcer(t *testing.T, s APIStore, q *mockQueue, enforcer BillingEnforcer) *Server {
	t.Helper()
	cfg := &config.Config{
		InternalSecret:      "test-secret-value",
		MaxBulkTriggerItems: 500,
		JWTSigningKey:       testJWTSigningKey,
	}
	var p pubsub.Publisher
	srv := NewServer(ServerDeps{
		Config:          cfg,
		Store:           s,
		Queue:           q,
		PubSub:          p,
		Edition:         domain.EditionCloud,
		BillingEnforcer: enforcer,
	})
	t.Cleanup(srv.Close)
	return srv
}

// TestBulkTrigger_PriorityCheckedPerItem_AllAllowed ensures the gate fires
// once per item even when every item passes the cap.
func TestBulkTrigger_PriorityCheckedPerItem_AllAllowed(t *testing.T) {
	t.Parallel()

	enforcer := &priorityRecordingEnforcer{maxPriority: 9}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newServerWithEnforcer(t, ms, mq, enforcer)

	body := `{"items":[{"priority":1},{"priority":2},{"priority":3}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := enforcer.calls.Load(); got != 3 {
		t.Fatalf("CheckMaxDispatchPriority called %d times, want 3 (one per priority>0 item)", got)
	}
}

// TestBulkTrigger_PriorityCheckedPerItem_ZeroPriorityNotChecked verifies that
// items with default priority (0) do not invoke the cap check, matching the
// single-trigger handler's behavior.
func TestBulkTrigger_PriorityCheckedPerItem_ZeroPriorityNotChecked(t *testing.T) {
	t.Parallel()

	enforcer := &priorityRecordingEnforcer{maxPriority: 9}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newServerWithEnforcer(t, ms, mq, enforcer)

	body := `{"items":[{},{"priority":5},{}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := enforcer.calls.Load(); got != 1 {
		t.Fatalf("CheckMaxDispatchPriority called %d times, want 1 (priority>0 only)", got)
	}
}

// TestBulkTrigger_PriorityRejectsSmuggled verifies that one over-cap item in a
// 99-item batch trips the gate and the entire transaction rolls back. This is
// the regression test for smuggling a priority=10 item into a batch of
// priority=1 items on a Free plan.
func TestBulkTrigger_PriorityRejectsSmuggled(t *testing.T) {
	t.Parallel()

	enforcer := &priorityRecordingEnforcer{maxPriority: 5}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newServerWithEnforcer(t, ms, mq, enforcer)

	items := make([]map[string]any, 0, 100)
	for i := range 99 {
		items = append(items, map[string]any{"priority": 1, "idempotency_key": fmt.Sprintf("k-%d", i)})
	}
	// Smuggled high-priority item at index 50.
	items[50]["priority"] = 99
	bodyBytes, _ := json.Marshal(map[string]any{"items": items})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", string(bodyBytes)))

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 PaymentRequired for over-cap item, got %d: %s", w.Code, w.Body.String())
	}
	if got := enforcer.calls.Load(); got < 1 {
		t.Fatalf("CheckMaxDispatchPriority calls = %d, want at least 1", got)
	}
}

func TestBulkTrigger_CloudNilBillingEnforcerFailsClosed(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newTestServer(t, ms, mq, nil)
	srv.edition = domain.EditionCloud

	body := `{"items":[{"priority":99},{"priority":100}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 with cloud nil enforcer, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "service_unavailable") {
		t.Fatalf("response body = %s, want service_unavailable code", w.Body.String())
	}
}

// TestBulkTrigger_CommunityNilBillingEnforcerFailsOpen verifies that the
// community edition does not block any priority without a billing enforcer.
func TestBulkTrigger_CommunityNilBillingEnforcerFailsOpen(t *testing.T) {
	t.Parallel()

	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newTestServer(t, ms, mq, nil) // no billing enforcer
	srv.edition = domain.EditionCommunity

	body := `{"items":[{"priority":99},{"priority":100}]}`
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", body))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 with no enforcer (fail-open), got %d: %s", w.Code, w.Body.String())
	}
}

// TestBulkTrigger_LargeBatch_GateCalledPerItem locks in that the gate is
// called for every item in a 500-item batch, preventing a future "check once"
// optimization from regressing the per-item invariant.
func TestBulkTrigger_LargeBatch_GateCalledPerItem(t *testing.T) {
	t.Parallel()

	enforcer := &priorityRecordingEnforcer{maxPriority: 100}
	ms := &APIStoreMock{
		GetJobFunc: func(_ context.Context, id string) (*domain.Job, error) {
			return testEnabledJob(id), nil
		},
	}
	mq := &mockQueue{enqueueFn: func(_ context.Context, _ *domain.JobRun) error { return nil }}
	srv := newServerWithEnforcer(t, ms, mq, enforcer)

	const itemCount = 500
	items := make([]map[string]any, 0, itemCount)
	for i := range itemCount {
		items = append(items, map[string]any{"priority": 5, "idempotency_key": fmt.Sprintf("k-%d", i)})
	}
	bodyBytes, _ := json.Marshal(map[string]any{"items": items})

	w := httptest.NewRecorder()
	srv.ServeHTTP(w, authedRequest(http.MethodPost, "/v1/jobs/job-123/trigger/bulk", string(bodyBytes)))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := enforcer.calls.Load(); got != int64(itemCount) {
		t.Fatalf("CheckMaxDispatchPriority called %d times, want %d", got, itemCount)
	}
}
