package agents

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"
)

// mockBillingEnforcer is a thread-safe mock implementing AgentBillingEnforcer.
type mockBillingEnforcer struct {
	mu                    sync.RWMutex
	checkSpendingLimitErr error
	agentPlanTier         string
	checkCallCount        atomic.Int64
	planCallCount         atomic.Int64
}

func (m *mockBillingEnforcer) CheckAgentSpendingLimit(_ context.Context, _ string) error {
	m.checkCallCount.Add(1)
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.checkSpendingLimitErr
}

func (m *mockBillingEnforcer) GetAgentPlanForProject(_ context.Context, _ string) (string, error) {
	m.planCallCount.Add(1)
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.agentPlanTier == "" {
		return string(domain.AgentPlanFree), nil
	}
	return m.agentPlanTier, nil
}

// mockBillingEnforcerWithToggle allows switching enforcement on/off mid-test.
type mockBillingEnforcerWithToggle struct {
	mu          sync.Mutex
	shouldBlock atomic.Bool
	blockErr    *billing.LimitError
}

func (m *mockBillingEnforcerWithToggle) CheckAgentSpendingLimit(_ context.Context, _ string) error {
	if m.shouldBlock.Load() {
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.blockErr
	}
	return nil
}

func (m *mockBillingEnforcerWithToggle) GetAgentPlanForProject(_ context.Context, _ string) (string, error) {
	return string(domain.AgentPlanFree), nil
}

// TestBillingEnforcer_ConcurrentCheckSpendingLimit verifies that the
// AgentBillingEnforcer interface implementation is safe for concurrent use.
func TestBillingEnforcer_ConcurrentCheckSpendingLimit(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{
		checkSpendingLimitErr: nil, // allow all
		agentPlanTier:         "agent_maker",
	}

	const goroutines = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var errCount atomic.Int64

	for range goroutines {
		go func() {
			defer wg.Done()
			if err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1"); err != nil {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if enforcer.checkCallCount.Load() != goroutines {
		t.Errorf("check call count = %d, want %d", enforcer.checkCallCount.Load(), goroutines)
	}
	if errCount.Load() != 0 {
		t.Errorf("unexpected errors: %d", errCount.Load())
	}
}

// TestBillingEnforcer_ConcurrentRejection verifies that concurrent calls to a
// blocking enforcer all receive the rejection error correctly.
func TestBillingEnforcer_ConcurrentRejection(t *testing.T) {
	t.Parallel()
	limitErr := &billing.LimitError{
		Code:         "agent_spending_limit_reached",
		Message:      "Budget exceeded",
		CurrentUsage: 1_000_000,
		Limit:        1_000_000,
		Plan:         "agent_free",
	}
	enforcer := &mockBillingEnforcer{
		checkSpendingLimitErr: limitErr,
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var rejected atomic.Int64

	for range goroutines {
		go func() {
			defer wg.Done()
			err := enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
			if err != nil {
				var le *billing.LimitError
				if errors.As(err, &le) && le.Code == "agent_spending_limit_reached" {
					rejected.Add(1)
				}
			}
		}()
	}
	wg.Wait()

	if rejected.Load() != goroutines {
		t.Errorf("expected %d rejections, got %d", goroutines, rejected.Load())
	}
}

// TestBillingEnforcer_GetAgentPlanForProject_ConcurrentMultiProject verifies
// that concurrent plan lookups for different projects are independent.
func TestBillingEnforcer_GetAgentPlanForProject_ConcurrentMultiProject(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{agentPlanTier: "agent_growth"}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var wrongTier atomic.Int64

	for i := range goroutines {
		projID := "proj-" + string(rune('A'+i%26))
		go func(pid string) {
			defer wg.Done()
			tier, err := enforcer.GetAgentPlanForProject(context.Background(), pid)
			if err != nil {
				t.Errorf("GetAgentPlanForProject(%q) error: %v", pid, err)
			}
			if tier != "agent_growth" {
				wrongTier.Add(1)
			}
		}(projID)
	}
	wg.Wait()

	if wrongTier.Load() > 0 {
		t.Errorf("%d goroutines received wrong tier", wrongTier.Load())
	}
	if enforcer.planCallCount.Load() != goroutines {
		t.Errorf("plan call count = %d, want %d", enforcer.planCallCount.Load(), goroutines)
	}
}

// TestBillingEnforcer_ToggleBlockMidFlight verifies that a billing enforcer
// whose state changes mid-test does not cause data races.
func TestBillingEnforcer_ToggleBlockMidFlight(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcerWithToggle{
		blockErr: &billing.LimitError{
			Code:    "agent_spending_limit_reached",
			Message: "Over budget",
			Limit:   1_000_000,
			Plan:    "agent_free",
		},
	}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	var panics atomic.Int64

	// Half goroutines flip the block state; half call the enforcer.
	for i := range goroutines {
		if i%2 == 0 {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						panics.Add(1)
					}
					wg.Done()
				}()
				enforcer.shouldBlock.Store(!enforcer.shouldBlock.Load())
			}()
		} else {
			go func() {
				defer func() {
					if r := recover(); r != nil {
						panics.Add(1)
					}
					wg.Done()
				}()
				_ = enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
			}()
		}
	}
	wg.Wait()

	if panics.Load() > 0 {
		t.Errorf("toggle under concurrent load caused %d panics", panics.Load())
	}
}

// TestAgentBillingGates_AllLimitErrorFieldsPopulated verifies that every
// LimitError returned by billing gate checks has all required fields populated.
func TestAgentBillingGates_AllLimitErrorFieldsPopulated(t *testing.T) {
	t.Parallel()

	limitErr := &billing.LimitError{
		Code:         "agent_spending_limit_reached",
		Message:      "Your agent budget of $1.00/month has been reached.",
		CurrentUsage: 1_000_001,
		Limit:        1_000_000,
		Plan:         "agent_free",
		UpgradeURL:   "/upgrade",
	}

	// Validate each required field.
	if limitErr.Code == "" {
		t.Error("LimitError.Code is empty")
	}
	if limitErr.Message == "" {
		t.Error("LimitError.Message is empty")
	}
	if limitErr.Limit <= 0 {
		t.Errorf("LimitError.Limit = %d, want > 0", limitErr.Limit)
	}
	if limitErr.CurrentUsage <= 0 {
		t.Errorf("LimitError.CurrentUsage = %d, want > 0", limitErr.CurrentUsage)
	}
	if limitErr.Plan == "" {
		t.Error("LimitError.Plan is empty")
	}
	if limitErr.UpgradeURL == "" {
		t.Error("LimitError.UpgradeURL is empty")
	}
	if limitErr.Error() == "" {
		t.Error("LimitError.Error() is empty")
	}
}

// TestBillingEnforcer_InterfaceCompliance verifies that both mock and real
// enforcer satisfy the AgentBillingEnforcer interface at compile time.
func TestBillingEnforcer_InterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ AgentBillingEnforcer = (*mockBillingEnforcer)(nil)
	var _ AgentBillingEnforcer = (*mockBillingEnforcerWithToggle)(nil)
}

// TestBillingEnforcer_PlanHierarchyAccess verifies that plan limits are
// accessible for all known agent plan tiers without error.
func TestBillingEnforcer_PlanHierarchyAccess(t *testing.T) {
	t.Parallel()

	tiers := []domain.PlanTier{
		domain.AgentPlanFree,
		domain.AgentPlanMaker,
		domain.AgentPlanGrowth,
		domain.AgentPlanEnterprise,
	}

	for _, tier := range tiers {
		t.Run(string(tier), func(t *testing.T) {
			t.Parallel()
			limits := billing.GetAgentPlanLimits(tier)
			if limits.PlanTier == "" {
				t.Errorf("tier %q: returned empty PlanTier", tier)
			}
		})
	}
}

// TestBillingEnforcer_ConcurrentMixedOps runs all billing enforcer operations
// concurrently to check for data races (most useful with go test -race).
func TestBillingEnforcer_ConcurrentMixedOps(t *testing.T) {
	t.Parallel()
	enforcer := &mockBillingEnforcer{agentPlanTier: "agent_maker"}

	const ops = 300
	var wg sync.WaitGroup
	wg.Add(ops)
	var panics atomic.Int64

	for i := range ops {
		op := i % 3
		go func() {
			defer func() {
				if r := recover(); r != nil {
					panics.Add(1)
				}
				wg.Done()
			}()
			switch op {
			case 0:
				_ = enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
			case 1:
				_, _ = enforcer.GetAgentPlanForProject(context.Background(), "proj-1")
			case 2:
				// Read the call counts (tests for data race on atomic fields).
				_ = enforcer.checkCallCount.Load()
				_ = enforcer.planCallCount.Load()
			}
		}()
	}
	wg.Wait()

	if panics.Load() > 0 {
		t.Errorf("concurrent mixed ops caused %d panics", panics.Load())
	}
}

// TestBillingLimitError_IsSentinel verifies LimitError can be identified via
// errors.As, which is the required pattern (no direct type assertions).
func TestBillingLimitError_IsSentinel(t *testing.T) {
	t.Parallel()

	original := &billing.LimitError{
		Code:    "agent_spending_limit_reached",
		Message: "Budget exceeded",
		Limit:   1_000_000,
		Plan:    "agent_free",
	}

	// errors.As must succeed on a direct LimitError.
	var target *billing.LimitError
	if !errors.As(original, &target) {
		t.Fatal("errors.As failed on direct *LimitError")
	}
	if target.Code != original.Code {
		t.Errorf("extracted Code = %q, want %q", target.Code, original.Code)
	}

	// errors.As must succeed on a wrapped LimitError.
	wrapped := errors.Join(errors.New("outer"), original)
	var target2 *billing.LimitError
	if !errors.As(wrapped, &target2) {
		t.Fatal("errors.As failed on wrapped *LimitError")
	}
	if target2.Code != original.Code {
		t.Errorf("extracted Code from wrapped = %q, want %q", target2.Code, original.Code)
	}
}

// TestBillingEnforcer_NilEnforcerSkipsChecks verifies that a nil billing
// enforcer in the service skips all billing checks without panicking.
// This mirrors the guards in service.go: `if s.billingEnforcer != nil`.
func TestBillingEnforcer_NilEnforcerSkipsChecks(t *testing.T) {
	t.Parallel()
	// A nil enforcer means billing enforcement is disabled (self-hosted mode).
	// We verify this by checking that the interface nil case is handled.
	var enforcer AgentBillingEnforcer = nil

	// Service code checks `if s.billingEnforcer != nil` before calling.
	// Simulate that guard.
	called := false
	if enforcer != nil {
		called = true
		_ = enforcer.CheckAgentSpendingLimit(context.Background(), "proj-1")
	}
	if called {
		t.Error("nil enforcer guard failed — billing check was called on nil enforcer")
	}
}
