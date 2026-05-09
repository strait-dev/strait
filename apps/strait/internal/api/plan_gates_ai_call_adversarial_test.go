package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// TestSDKUsage_RaceAtCap proves that with N+1 concurrent calls against a Free
// org sitting at the cap, exactly the cap-delta number of calls succeed and
// the rest are rejected. The enforcer's atomic INCR script is the authority
// here; the mock simulates the script's behavior so we can run this test
// without Redis.
func TestSDKUsage_RaceAtCap(t *testing.T) {
	t.Parallel()

	const cap = int64(5)

	used := atomic.Int64{}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting, Attempt: 1}, nil
		},
		CreateRunUsageFunc: func(_ context.Context, _ *domain.RunUsage) error {
			return nil
		},
	}
	// Simulate atomic INCR-then-check: each call increments; if used > cap,
	// reject and decrement. Mirrors the Lua script's effect.
	enforcer := &raceAIEnforcer{
		cap:  cap,
		used: &used,
	}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	const total = 12
	results := make(chan int, total)
	var wg sync.WaitGroup
	for range total {
		wg.Go(func() {
			w := httptest.NewRecorder()
			r := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1", sdkUsageBody())
			srv.ServeHTTP(w, r)
			results <- w.Code
		})
	}
	wg.Wait()
	close(results)

	accepted, rejected := 0, 0
	for code := range results {
		switch code {
		case http.StatusCreated:
			accepted++
		case http.StatusTooManyRequests:
			rejected++
		default:
			t.Errorf("unexpected status %d", code)
		}
	}
	if int64(accepted) != cap {
		t.Errorf("accepted = %d; want exactly %d (cap)", accepted, cap)
	}
	if accepted+rejected != total {
		t.Errorf("accepted+rejected (%d) != total (%d)", accepted+rejected, total)
	}
}

type raceAIEnforcer struct {
	mockBillingEnforcer
	cap  int64
	used *atomic.Int64
}

func (r *raceAIEnforcer) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	return "org-1", nil
}

func (r *raceAIEnforcer) GetActiveProjectOrgID(_ context.Context, _ string) (string, error) {
	return "org-1", nil
}

func (r *raceAIEnforcer) CheckDailyAIModelCallLimit(_ context.Context, _ string) error {
	cur := r.used.Add(1)
	if cur > r.cap {
		r.used.Add(-1)
		return &billing.LimitError{
			Code:         "org_daily_ai_call_limit_exceeded",
			Message:      "Your Free plan allows N AI model calls per day. You've used N+1.",
			Limit:        r.cap,
			CurrentUsage: cur - 1,
			Plan:         string(domain.PlanFree),
		}
	}
	return nil
}

// TestSDKUsage_TokenReusePostDowngrade_GateReevaluates locks in the contract
// that the gate is consulted on every request, never cached behind the run
// token. A run token issued while the org was Pro must not bypass a gate
// after the org downgrades to Free mid-execution.
func TestSDKUsage_TokenReusePostDowngrade_GateReevaluates(t *testing.T) {
	t.Parallel()

	gateCalls := atomic.Int64{}
	ms := &APIStoreMock{
		GetRunFunc: func(_ context.Context, id string) (*domain.JobRun, error) {
			return &domain.JobRun{ID: id, JobID: "job-1", ProjectID: "proj-1", Status: domain.StatusExecuting, Attempt: 1}, nil
		},
		CreateRunUsageFunc: func(_ context.Context, _ *domain.RunUsage) error { return nil },
	}
	enforcer := &flippingAIEnforcer{calls: &gateCalls}
	srv := newServerWithEnforcer(t, ms, &mockQueue{}, enforcer)

	// First request: enforcer reports below cap.
	w1 := httptest.NewRecorder()
	r1 := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1", sdkUsageBody())
	srv.ServeHTTP(w1, r1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first request expected 201, got %d", w1.Code)
	}

	// Simulate downgrade: enforcer flips to over-cap state.
	enforcer.over.Store(true)

	// Second request reuses the same run token; the gate must re-evaluate
	// against the new plan state and reject.
	w2 := httptest.NewRecorder()
	r2 := sdkRequest(t, http.MethodPost, "/sdk/v1/runs/run-1/usage", "run-1", sdkUsageBody())
	srv.ServeHTTP(w2, r2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request after simulated downgrade expected 429, got %d: %s", w2.Code, w2.Body.String())
	}

	if got := gateCalls.Load(); got != 2 {
		t.Errorf("gate must fire on every request; calls=%d, want 2", got)
	}
}

type flippingAIEnforcer struct {
	mockBillingEnforcer
	over  atomic.Bool
	calls *atomic.Int64
}

func (f *flippingAIEnforcer) GetProjectOrgID(_ context.Context, _ string) (string, error) {
	return "org-1", nil
}

func (f *flippingAIEnforcer) GetActiveProjectOrgID(_ context.Context, _ string) (string, error) {
	return "org-1", nil
}

func (f *flippingAIEnforcer) CheckDailyAIModelCallLimit(_ context.Context, _ string) error {
	f.calls.Add(1)
	if f.over.Load() {
		return &billing.LimitError{Code: "org_daily_ai_call_limit_exceeded", Message: "over cap", Plan: string(domain.PlanFree)}
	}
	return nil
}
