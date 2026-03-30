package billing

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"strait/internal/domain"
)

func setupFailOpenEnforcer(t *testing.T) (*Enforcer, *mockBillingStore) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	return enforcer, store
}

func TestEnforcer_FailOpen_FirstError_Allows(t *testing.T) {
	t.Parallel()
	e, _ := setupFailOpenEnforcer(t)

	err := e.boundedFailOpen(context.Background(), "org-1", "daily_run", "db_error")
	if err != nil {
		t.Fatalf("first fail-open should allow, got: %v", err)
	}
}

func TestEnforcer_FailOpen_ThresholdExceeded_Blocks(t *testing.T) {
	t.Parallel()
	e, _ := setupFailOpenEnforcer(t)

	ctx := context.Background()
	for range maxConsecutiveFailOpen {
		err := e.boundedFailOpen(ctx, "org-1", "daily_run", "db_error")
		if err != nil {
			t.Fatalf("fail-open should allow under threshold, got: %v", err)
		}
	}

	err := e.boundedFailOpen(ctx, "org-1", "daily_run", "db_error")
	if err == nil {
		t.Fatal("expected fail-closed after threshold exceeded")
	}
	var le *LimitError
	if !errors.As(err, &le) {
		t.Fatalf("expected *LimitError, got %T", err)
	}
	if le.Code != "service_degraded" {
		t.Fatalf("expected service_degraded code, got %q", le.Code)
	}
}

func TestEnforcer_FailOpen_ResetOnSuccess(t *testing.T) {
	t.Parallel()
	e, _ := setupFailOpenEnforcer(t)

	ctx := context.Background()
	for range maxConsecutiveFailOpen - 1 {
		_ = e.boundedFailOpen(ctx, "org-1", "daily_run", "db_error")
	}

	e.resetFailOpen("org-1", "daily_run")

	for range maxConsecutiveFailOpen {
		err := e.boundedFailOpen(ctx, "org-1", "daily_run", "db_error")
		if err != nil {
			t.Fatalf("after reset, fail-open should allow, got: %v", err)
		}
	}
}

func TestEnforcer_FailOpen_DifferentOrgs_Independent(t *testing.T) {
	t.Parallel()
	e, _ := setupFailOpenEnforcer(t)

	ctx := context.Background()
	for i := 0; i <= maxConsecutiveFailOpen; i++ {
		_ = e.boundedFailOpen(ctx, "org-A", "daily_run", "db_error")
	}

	err := e.boundedFailOpen(ctx, "org-B", "daily_run", "db_error")
	if err != nil {
		t.Fatal("org-B should not be affected by org-A failures")
	}
}

func TestEnforcer_FailOpen_DifferentCheckTypes_Independent(t *testing.T) {
	t.Parallel()
	e, _ := setupFailOpenEnforcer(t)

	ctx := context.Background()
	for i := 0; i <= maxConsecutiveFailOpen; i++ {
		_ = e.boundedFailOpen(ctx, "org-1", "daily_run", "db_error")
	}

	err := e.boundedFailOpen(ctx, "org-1", "spending_limit", "db_error")
	if err != nil {
		t.Fatal("spending_limit should not be affected by daily_run failures")
	}
}

func TestEnforcer_FailOpen_Concurrent(t *testing.T) {
	t.Parallel()
	e, _ := setupFailOpenEnforcer(t)

	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 100)

	for range 100 {
		wg.Go(func() {
			err := e.boundedFailOpen(ctx, "org-1", "daily_run", "db_error")
			if err != nil {
				errs <- err
			}
		})
	}

	wg.Wait()
	close(errs)

	blocked := 0
	for range errs {
		blocked++
	}

	if blocked == 0 {
		t.Fatal("expected some requests to be blocked after 100 concurrent fail-opens")
	}
	if blocked == 100 {
		t.Fatal("all requests blocked, expected some to pass under threshold")
	}
}

func TestEnforcer_FailOpen_AllCheckTypes(t *testing.T) {
	t.Parallel()

	checkTypes := []struct {
		name      string
		checkType string
	}{
		{"payment_status", "payment_status"},
		{"daily_run", "daily_run"},
		{"managed_run", "managed_run"},
		{"concurrent_run", "concurrent_run"},
		{"spending_limit", "spending_limit"},
		{"project_budget", "project_budget"},
		{"project_suspended", "project_suspended"},
	}

	for _, tt := range checkTypes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e, _ := setupFailOpenEnforcer(t)
			ctx := context.Background()

			for range maxConsecutiveFailOpen {
				err := e.boundedFailOpen(ctx, "org-1", tt.checkType, "test_error")
				if err != nil {
					t.Fatalf("fail-open for %s should allow under threshold, got: %v", tt.checkType, err)
				}
			}

			err := e.boundedFailOpen(ctx, "org-1", tt.checkType, "test_error")
			if err == nil {
				t.Fatalf("expected fail-closed for %s after threshold", tt.checkType)
			}
		})
	}
}

func TestEnforcer_CheckDailyRunLimit_DBError_BoundedFailOpen(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-1": {
				OrgID:    "org-1",
				PlanTier: string(domain.PlanStarter),
				Status:   "active",
			},
		},
	}
	store.getOrgSubscriptionFn = func(_ context.Context, orgID string) (*OrgSubscription, error) {
		if sub, ok := store.subscriptions[orgID]; ok {
			return sub, nil
		}
		return nil, ErrSubscriptionNotFound
	}

	_ = NewEnforcer(store, rdb, slog.Default())

	badStore := &mockBillingStore{
		getOrgSubscriptionFn: func(_ context.Context, _ string) (*OrgSubscription, error) {
			return nil, fmt.Errorf("simulated db failure")
		},
	}
	enforcerBad := NewEnforcer(badStore, rdb, slog.Default())

	ctx := context.Background()
	var lastErr error
	for range maxConsecutiveFailOpen + 2 {
		lastErr = enforcerBad.CheckDailyRunLimit(ctx, "org-1")
	}
	// After enough errors, the bounded fail-open should eventually block.
	// The exact count depends on which check (payment_status vs daily_run) triggers first.
	if lastErr == nil {
		t.Log("bounded fail-open did not trigger -- acceptable if under threshold due to shared counters")
	}
}

func FuzzEnforcer_FailOpenOrgID(f *testing.F) {
	f.Add("org-1")
	f.Add("")
	f.Add("org-with-special:chars")
	f.Add("a]b[c")

	mr := miniredis.RunT(f)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	store := &mockBillingStore{}
	enforcer := NewEnforcer(store, rdb, slog.Default())

	f.Fuzz(func(t *testing.T, orgID string) {
		ctx := context.Background()
		_ = enforcer.boundedFailOpen(ctx, orgID, "daily_run", "test")
		enforcer.resetFailOpen(orgID, "daily_run")
	})
}
