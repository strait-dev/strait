package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// aiCallEnforcer extends mockBillingEnforcer to record CheckDailyAIModelCallLimit
// invocations and return a configurable error so the test can simulate the
// enforcer's Free-tier hard reject without spinning up Redis.
type aiCallEnforcer struct {
	mockBillingEnforcer
	calls atomic.Int64
	rej   *billing.LimitError
	orgID string
}

func (a *aiCallEnforcer) CheckDailyAIModelCallLimit(_ context.Context, _ string) error {
	a.calls.Add(1)
	if a.rej != nil {
		return a.rej
	}
	return nil
}

func (a *aiCallEnforcer) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	if a.orgID == "" {
		return "org-1", nil
	}
	return a.orgID, nil
}

func (a *aiCallEnforcer) GetActiveProjectOrgID(ctx context.Context, projectID string) (string, error) {
	return a.GetProjectOrgID(ctx, projectID)
}

func sdkUsageBody() string {
	return `{"provider":"openai","model":"gpt-4","prompt_tokens":10,"completion_tokens":5}`
}

// TestSDKUsage_FreeTierAtCap_Rejects proves the gate fires before the row is
// recorded when the enforcer reports Free-tier overage. The 429 status mirrors
// other rate-limit rejections in the SDK plane.
func TestSDKUsage_FreeTierAtCap_Rejects(t *testing.T) {
	t.Parallel()

	createCalls := atomic.Int64{}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting, Attempt: 1}, nil
		},
		CreateRunUsageFunc: func(_ context.Context, _ *domain.RunUsage) error {
			createCalls.Add(1)
			return nil
		},
	}
	enforcer := &aiCallEnforcer{
		rej: &billing.LimitError{
			Code:         "org_daily_ai_call_limit_exceeded",
			Message:      "Your Free plan allows 20 AI model calls per day. You've used 21.",
			Limit:        20,
			CurrentUsage: 21,
			Plan:         string(domain.PlanFree),
		},
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1", sdkUsageBody())
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 over cap, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "AI model calls per day") {
		t.Errorf("rejection body must surface the daily-call limit message, got: %s", w.Body.String())
	}
	if got := createCalls.Load(); got != 0 {
		t.Errorf("CreateRunUsage called %d times after gate rejection; want 0", got)
	}
	if got := enforcer.calls.Load(); got != 1 {
		t.Errorf("CheckDailyAIModelCallLimit calls = %d; want 1", got)
	}
}

// TestSDKUsage_BelowCap_Records confirms the happy path: the enforcer returns
// nil and the usage row is created.
func TestSDKUsage_BelowCap_Records(t *testing.T) {
	t.Parallel()

	createCalls := atomic.Int64{}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting, Attempt: 1}, nil
		},
		CreateRunUsageFunc: func(_ context.Context, _ *domain.RunUsage) error {
			createCalls.Add(1)
			return nil
		},
	}
	enforcer := &aiCallEnforcer{}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1", sdkUsageBody())
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if got := createCalls.Load(); got != 1 {
		t.Errorf("CreateRunUsage calls = %d; want 1", got)
	}
}

// TestSDKUsage_NilEnforcer_FailsOpen ensures a self-hosted build with no
// billing enforcer always records.
func TestSDKUsage_NilEnforcer_FailsOpen(t *testing.T) {
	t.Parallel()

	createCalls := atomic.Int64{}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting, Attempt: 1}, nil
		},
		CreateRunUsageFunc: func(_ context.Context, _ *domain.RunUsage) error {
			createCalls.Add(1)
			return nil
		},
	}
	srv := newTestServer(t, ms, &mockQueue{}, nil)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1", sdkUsageBody())
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("nil enforcer must fail open; got %d: %s", w.Code, w.Body.String())
	}
	if got := createCalls.Load(); got != 1 {
		t.Errorf("CreateRunUsage calls = %d; want 1", got)
	}
}

// TestSDKUsage_RunLookupFails_FailsOpen ensures a transient store failure on
// the GetRun lookup does not drop usage telemetry.
func TestSDKUsage_RunLookupFails_FailsOpen(t *testing.T) {
	t.Parallel()

	createCalls := atomic.Int64{}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, _ string) (*domain.JobRun, error) {
			return nil, nil
		},
		CreateRunUsageFunc: func(_ context.Context, _ *domain.RunUsage) error {
			createCalls.Add(1)
			return nil
		},
	}
	enforcer := &aiCallEnforcer{}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	w := httptest.NewRecorder()
	r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1", sdkUsageBody())
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("run lookup miss must fail open; got %d: %s", w.Code, w.Body.String())
	}
	if got := enforcer.calls.Load(); got != 0 {
		t.Errorf("enforcer must not be called when run lookup misses; calls=%d", got)
	}
}
