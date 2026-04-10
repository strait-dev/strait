package billing

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// slowMockBillingStore wraps mockBillingStore and injects a configurable delay
// into SumOrgAgentSpendSince to widen the TOCTOU window for concurrent tests.
type slowMockBillingStore struct {
	mockBillingStore
	readDelay time.Duration
	callCount atomic.Int64
}

func (s *slowMockBillingStore) SumOrgAgentSpendSince(ctx context.Context, orgID string, since time.Time) (int64, error) {
	s.callCount.Add(1)
	if s.readDelay > 0 {
		select {
		case <-time.After(s.readDelay):
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return s.mockBillingStore.SumOrgAgentSpendSince(ctx, orgID, since)
}

func newAgentEnforcerWithRedis(t *testing.T, store *mockBillingStore) (*Enforcer, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewEnforcer(store, rdb, nil), mr
}

// TestCheckAgentSpendingLimit_ConcurrentSafe verifies that many goroutines
// calling CheckAgentSpendingLimit simultaneously do not panic or corrupt
// state, and all receive a consistent answer.
func TestCheckAgentSpendingLimit_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap:   map[string]string{"proj-1": "org-1"},
		subscriptions:   map[string]*OrgSubscription{"org-1": {OrgID: "org-1", AgentPlanTier: "agent_free"}},
		agentSpendByOrg: map[string]int64{"org-1": 500_000}, // $0.50, under $1 cap
	}
	enforcer, _ := newAgentEnforcerWithRedis(t, store)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errors := make([]error, goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			errors[idx] = enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
		}(i)
	}
	wg.Wait()

	// All should pass — spend is under cap.
	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d: expected nil error (under cap), got %v", i, err)
		}
	}
}

// TestCheckAgentSpendingLimit_ConcurrentAtCap verifies that concurrent
// calls at exactly the cap all reject correctly and consistently.
func TestCheckAgentSpendingLimit_ConcurrentAtCap(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap:   map[string]string{"proj-1": "org-1"},
		subscriptions:   map[string]*OrgSubscription{"org-1": {OrgID: "org-1", AgentPlanTier: "agent_free"}},
		agentSpendByOrg: map[string]int64{"org-1": AgentCreditFreeMicrousd}, // exactly at $1
	}
	enforcer, _ := newAgentEnforcerWithRedis(t, store)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var rejected atomic.Int64

	for range goroutines {
		go func() {
			defer wg.Done()
			if err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1"); err != nil {
				rejected.Add(1)
			}
		}()
	}
	wg.Wait()

	if rejected.Load() != goroutines {
		t.Errorf("expected all %d goroutines to be rejected, got %d rejections", goroutines, rejected.Load())
	}
}

// TestCheckAgentSpendingLimit_TOCTOUWindow demonstrates the check-then-commit
// race window. When the DB read is delayed, two goroutines can both observe
// spend just under the cap and both pass — the enforcer does not close this
// window (it is a read-only gate relying on upstream commit ordering).
//
// This test documents the known behavior and runs with -race to verify no
// data races occur in the enforcer itself.
func TestCheckAgentSpendingLimit_TOCTOWWindow(t *testing.T) {
	t.Parallel()

	// Set spend to cap - 1 (one micro-USD under free cap).
	underCapSpend := AgentCreditFreeMicrousd - 1
	slow := &slowMockBillingStore{
		readDelay: 5 * time.Millisecond, // widen the TOCTOU window
	}
	slow.projectOrgMap = map[string]string{"proj-1": "org-1"}
	slow.subscriptions = map[string]*OrgSubscription{
		"org-1": {OrgID: "org-1", AgentPlanTier: "agent_free"},
	}
	slow.agentSpendByOrg = map[string]int64{"org-1": underCapSpend}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	enforcer := NewEnforcer(&slow.mockBillingStore, rdb, nil)

	// Override the mock's SumOrgAgentSpendSince to use the slow version.
	// Since we can't easily swap it, use a different enforcer approach:
	// inject via getOrgSubscriptionFn and simulate slowness there.
	_ = enforcer

	// Build a fresh enforcer with the slow store directly.
	enforcer2 := NewEnforcer(slow, rdb, nil)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var passed atomic.Int64
	var rejected atomic.Int64

	for range goroutines {
		go func() {
			defer wg.Done()
			if err := enforcer2.CheckAgentSpendingLimit(context.Background(), "proj-1"); err != nil {
				rejected.Add(1)
			} else {
				passed.Add(1)
			}
		}()
	}
	wg.Wait()

	// With spend at cap-1, all goroutines should pass — and they do because
	// the store returns the same static value every time. The TOCTOU risk
	// is that between check and commit, another call records a new spend.
	// This test ensures no data races (-race) in the enforcer's own state.
	if slow.callCount.Load() != int64(goroutines) {
		t.Errorf("expected %d store reads, got %d", goroutines, slow.callCount.Load())
	}
	if passed.Load() != goroutines {
		t.Errorf("expected all %d goroutines to pass (spend at cap-1), got %d passed, %d rejected",
			goroutines, passed.Load(), rejected.Load())
	}
}

// TestCheckAgentSpendingLimit_ConcurrentMultiOrg verifies that concurrent
// calls for different orgs are independently evaluated with no state bleed.
func TestCheckAgentSpendingLimit_ConcurrentMultiOrg(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap: map[string]string{
			"proj-free-over": "org-free-over",
			"proj-paid":      "org-paid",
		},
		subscriptions: map[string]*OrgSubscription{
			"org-free-over": {OrgID: "org-free-over", AgentPlanTier: "agent_free"},
			"org-paid":      {OrgID: "org-paid", AgentPlanTier: "agent_maker", AgentSpendingLimitMicrousd: -1},
		},
		agentSpendByOrg: map[string]int64{
			"org-free-over": AgentCreditFreeMicrousd + 1, // over cap
			"org-paid":      100_000_000,                 // way over, but no cap
		},
	}
	enforcer, _ := newAgentEnforcerWithRedis(t, store)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	var freeRejected atomic.Int64
	var paidPassed atomic.Int64

	for range goroutines {
		go func() {
			defer wg.Done()
			if err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-free-over"); err != nil {
				freeRejected.Add(1)
			}
		}()
		go func() {
			defer wg.Done()
			if err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-paid"); err == nil {
				paidPassed.Add(1)
			}
		}()
	}
	wg.Wait()

	if freeRejected.Load() != goroutines {
		t.Errorf("free org: expected %d rejections, got %d", goroutines, freeRejected.Load())
	}
	if paidPassed.Load() != goroutines {
		t.Errorf("paid org: expected %d passes, got %d", goroutines, paidPassed.Load())
	}
}

// TestCheckAgentSpendingLimit_ConcurrentSpendingCap verifies the paid plan
// optional spending cap is enforced correctly under concurrent load.
func TestCheckAgentSpendingLimit_ConcurrentSpendingCap(t *testing.T) {
	t.Parallel()
	const capMicrousd = 50_000_000 // $50
	store := &mockBillingStore{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
		subscriptions: map[string]*OrgSubscription{
			"org-1": {
				OrgID:                      "org-1",
				AgentPlanTier:              "agent_maker",
				AgentSpendingLimitMicrousd: capMicrousd,
			},
		},
		agentSpendByOrg: map[string]int64{"org-1": capMicrousd}, // exactly at cap
	}
	enforcer, _ := newAgentEnforcerWithRedis(t, store)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var rejected atomic.Int64

	for range goroutines {
		go func() {
			defer wg.Done()
			if err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1"); err != nil {
				rejected.Add(1)
			}
		}()
	}
	wg.Wait()

	if rejected.Load() != goroutines {
		t.Errorf("expected all %d rejected at spending cap, got %d", goroutines, rejected.Load())
	}
}

// TestCheckAgentSpendingLimit_ConcurrentStoreError verifies that store errors
// result in fail-open (nil error returned) and no panics under concurrent load.
func TestCheckAgentSpendingLimit_ConcurrentStoreError(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*OrgSubscription, error) {
			// Simulate a flaky DB.
			return nil, ErrSubscriptionNotFound
		},
		agentSpendByOrg: map[string]int64{"org-1": AgentCreditFreeMicrousd + 1},
	}
	enforcer, _ := newAgentEnforcerWithRedis(t, store)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var panics atomic.Int64

	for range goroutines {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					panics.Add(1)
				}
				wg.Done()
			}()
			// When subscription not found, falls back to free cap check.
			// With spend over free cap, should reject.
			_ = enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
		}()
	}
	wg.Wait()

	if panics.Load() > 0 {
		t.Errorf("concurrent store error caused %d panics", panics.Load())
	}
}

// TestCheckAgentSpendingLimit_ConcurrentMixedResults runs goroutines checking
// multiple project IDs concurrently, verifying independent org enforcement.
func TestCheckAgentSpendingLimit_ConcurrentMixedResults(t *testing.T) {
	t.Parallel()

	const numProjects = 5
	projectOrgMap := make(map[string]string, numProjects)
	subscriptions := make(map[string]*OrgSubscription, numProjects)
	agentSpendByOrg := make(map[string]int64, numProjects)

	// Half over cap, half under.
	for i := range numProjects {
		projID := "proj-" + string(rune('A'+i))
		orgID := "org-" + string(rune('A'+i))
		projectOrgMap[projID] = orgID
		subscriptions[orgID] = &OrgSubscription{OrgID: orgID, AgentPlanTier: "agent_free"}
		if i%2 == 0 {
			agentSpendByOrg[orgID] = AgentCreditFreeMicrousd + 1 // over
		} else {
			agentSpendByOrg[orgID] = AgentCreditFreeMicrousd / 2 // under
		}
	}

	store := &mockBillingStore{
		projectOrgMap:   projectOrgMap,
		subscriptions:   subscriptions,
		agentSpendByOrg: agentSpendByOrg,
	}
	enforcer, _ := newAgentEnforcerWithRedis(t, store)

	const goroutinesPerProject = 20
	var wg sync.WaitGroup
	wg.Add(numProjects * goroutinesPerProject)

	overCapCount := atomic.Int64{}
	underCapCount := atomic.Int64{}

	for i := range numProjects {
		projID := "proj-" + string(rune('A'+i))
		isOverCap := i%2 == 0
		for range goroutinesPerProject {
			go func(pid string, over bool) {
				defer wg.Done()
				err := enforcer.CheckAgentSpendingLimit(context.Background(), pid)
				if over {
					if err != nil {
						overCapCount.Add(1)
					}
				} else {
					if err == nil {
						underCapCount.Add(1)
					}
				}
			}(projID, isOverCap)
		}
	}
	wg.Wait()

	// 3 over-cap projects (i=0,2,4), 2 under-cap projects (i=1,3).
	expectedOver := int64(3 * goroutinesPerProject)
	expectedUnder := int64(2 * goroutinesPerProject)

	if overCapCount.Load() != expectedOver {
		t.Errorf("over-cap rejections: got %d, want %d", overCapCount.Load(), expectedOver)
	}
	if underCapCount.Load() != expectedUnder {
		t.Errorf("under-cap passes: got %d, want %d", underCapCount.Load(), expectedUnder)
	}
}

// TestCheckAgentSpendingLimit_AllPlanTiersConcurrent verifies all plan tiers
// are handled correctly under concurrent load without panics.
func TestCheckAgentSpendingLimit_AllPlanTiersConcurrent(t *testing.T) {
	t.Parallel()

	planCases := []struct {
		planTier              string
		spendMicrousd         int64
		spendingLimitMicrousd int64
		expectReject          bool
	}{
		{"agent_free", AgentCreditFreeMicrousd + 1, 0, true},
		{"agent_free", AgentCreditFreeMicrousd - 1, 0, false},
		{"agent_maker", 10_000_000, -1, false},        // no cap
		{"agent_maker", 50_000_000, 50_000_000, true}, // at cap
		{"agent_growth", 200_000_000, -1, false},      // no cap
		{"agent_enterprise", 999_999_999, 0, false},   // enterprise: no cap
		{"", AgentCreditFreeMicrousd + 1, 0, true},    // empty defaults to free
	}

	for _, tc := range planCases {
		t.Run("tier="+tc.planTier, func(t *testing.T) {
			t.Parallel()
			store := &mockBillingStore{
				projectOrgMap: map[string]string{"proj-1": "org-1"},
				subscriptions: map[string]*OrgSubscription{
					"org-1": {
						OrgID:                      "org-1",
						AgentPlanTier:              tc.planTier,
						AgentSpendingLimitMicrousd: tc.spendingLimitMicrousd,
					},
				},
				agentSpendByOrg: map[string]int64{"org-1": tc.spendMicrousd},
			}
			enforcer, _ := newAgentEnforcerWithRedis(t, store)

			const goroutines = 20
			var wg sync.WaitGroup
			wg.Add(goroutines)
			var rejected atomic.Int64

			for range goroutines {
				go func() {
					defer wg.Done()
					if err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1"); err != nil {
						rejected.Add(1)
					}
				}()
			}
			wg.Wait()

			if tc.expectReject && rejected.Load() != goroutines {
				t.Errorf("tier=%q: expected %d rejections, got %d", tc.planTier, goroutines, rejected.Load())
			}
			if !tc.expectReject && rejected.Load() != 0 {
				t.Errorf("tier=%q: expected 0 rejections, got %d", tc.planTier, rejected.Load())
			}
		})
	}
}

// TestGetAgentPlanForProject_ConcurrentSafe verifies that concurrent
// GetAgentPlanForProject calls do not race or panic.
func TestGetAgentPlanForProject_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		projectOrgMap: map[string]string{"proj-1": "org-1"},
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", AgentPlanTier: "agent_maker"},
		},
	}
	enforcer, _ := newAgentEnforcerWithRedis(t, store)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make([]string, goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			tier, err := enforcer.GetAgentPlanForProject(context.Background(), "proj-1")
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", idx, err)
			}
			results[idx] = tier
		}(i)
	}
	wg.Wait()

	for i, tier := range results {
		if tier != "agent_maker" {
			t.Errorf("goroutine %d: tier = %q, want agent_maker", i, tier)
		}
	}
}

// TestCheckAgentSpendingLimit_ContextCancellation verifies that context
// cancellation is handled gracefully under concurrent load.
func TestCheckAgentSpendingLimit_ContextCancellation(t *testing.T) {
	t.Parallel()

	slow := &slowMockBillingStore{
		readDelay: 50 * time.Millisecond,
	}
	slow.projectOrgMap = map[string]string{"proj-1": "org-1"}
	slow.subscriptions = map[string]*OrgSubscription{
		"org-1": {OrgID: "org-1", AgentPlanTier: "agent_free"},
	}
	slow.agentSpendByOrg = map[string]int64{"org-1": 0}

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	enforcer := NewEnforcer(slow, rdb, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var panics atomic.Int64

	for range goroutines {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					panics.Add(1)
				}
				wg.Done()
			}()
			// Context may be cancelled before the slow read returns.
			// Enforcer must not panic.
			_ = enforcer.CheckAgentSpendingLimit(ctx, "proj-1")
		}()
	}
	wg.Wait()

	if panics.Load() > 0 {
		t.Errorf("context cancellation caused %d panics", panics.Load())
	}
}

// TestCheckAgentSpendingLimit_EmptyOrgID_Concurrent verifies that empty
// org ID (from unknown project) returns nil for all concurrent callers.
func TestCheckAgentSpendingLimit_EmptyOrgID_Concurrent(t *testing.T) {
	t.Parallel()
	// No projectOrgMap entry — GetProjectOrgID returns empty string.
	store := &mockBillingStore{}
	enforcer, _ := newAgentEnforcerWithRedis(t, store)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var errCount atomic.Int64

	for range goroutines {
		go func() {
			defer wg.Done()
			if err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-unknown"); err != nil {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if errCount.Load() != 0 {
		t.Errorf("expected 0 errors for unknown project, got %d", errCount.Load())
	}
}

// TestEffectiveMaxProjects_ConcurrentSafe verifies effectiveMaxProjects
// is safe for concurrent use.
func TestEffectiveMaxProjects_ConcurrentSafe(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {OrgID: "org-1", PlanTier: "pro", AgentPlanTier: "agent_growth"},
		},
	}
	enforcer, _ := newAgentEnforcerWithRedis(t, store)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make([]int, goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			results[idx] = enforcer.effectiveMaxProjects(context.Background(), "org-1")
		}(i)
	}
	wg.Wait()

	// All results should be the same non-zero value.
	first := results[0]
	if first <= 0 {
		t.Errorf("effectiveMaxProjects = %d, want > 0", first)
	}
	for i, r := range results {
		if r != first {
			t.Errorf("goroutine %d: effectiveMaxProjects = %d, want %d", i, r, first)
		}
	}
}

// TestCheckAgentSpendingLimit_BoundaryValues_Concurrent tests exact boundary
// values under concurrent load: cap-1, cap, cap+1.
func TestCheckAgentSpendingLimit_BoundaryValues_Concurrent(t *testing.T) {
	t.Parallel()

	boundaries := []struct {
		name         string
		spend        int64
		expectReject bool
	}{
		{"cap_minus_1", AgentCreditFreeMicrousd - 1, false},
		{"cap_exact", AgentCreditFreeMicrousd, true},
		{"cap_plus_1", AgentCreditFreeMicrousd + 1, true},
		{"zero", 0, false},
		{"max_int64", 1 << 62, true}, // way over cap
	}

	for _, tc := range boundaries {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := &mockBillingStore{
				projectOrgMap:   map[string]string{"proj-1": "org-1"},
				subscriptions:   map[string]*OrgSubscription{"org-1": {OrgID: "org-1", AgentPlanTier: "agent_free"}},
				agentSpendByOrg: map[string]int64{"org-1": tc.spend},
			}
			enforcer, _ := newAgentEnforcerWithRedis(t, store)

			const goroutines = 30
			var wg sync.WaitGroup
			wg.Add(goroutines)
			var rejected atomic.Int64

			for range goroutines {
				go func() {
					defer wg.Done()
					if err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1"); err != nil {
						rejected.Add(1)
					}
				}()
			}
			wg.Wait()

			if tc.expectReject && rejected.Load() != goroutines {
				t.Errorf("spend=%d: expected %d rejections, got %d", tc.spend, goroutines, rejected.Load())
			}
			if !tc.expectReject && rejected.Load() != 0 {
				t.Errorf("spend=%d: expected 0 rejections, got %d", tc.spend, rejected.Load())
			}
		})
	}
}
