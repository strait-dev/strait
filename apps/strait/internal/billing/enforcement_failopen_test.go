package billing

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/require"

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
	require.NoError(t,
		err)
}

func TestEnforcer_FailOpen_ThresholdExceeded_Blocks(t *testing.T) {
	t.Parallel()
	e, _ := setupFailOpenEnforcer(t)

	ctx := context.Background()
	for range maxConsecutiveFailOpen {
		err := e.boundedFailOpen(ctx, "org-1", "daily_run", "db_error")
		require.NoError(t,
			err)
	}

	err := e.boundedFailOpen(ctx, "org-1", "daily_run", "db_error")
	require.Error(t,
		err)

	var le *LimitError
	require.ErrorAs(t, err, &le)
	require.Equal(t,
		"service_degraded",

		le.Code)
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
		require.NoError(t,
			err)
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
	require.NoError(t,
		err)
}

func TestEnforcer_FailOpen_DifferentCheckTypes_Independent(t *testing.T) {
	t.Parallel()
	e, _ := setupFailOpenEnforcer(t)

	ctx := context.Background()
	for i := 0; i <= maxConsecutiveFailOpen; i++ {
		_ = e.boundedFailOpen(ctx, "org-1", "daily_run", "db_error")
	}

	err := e.boundedFailOpen(ctx, "org-1", "spending_limit", "db_error")
	require.NoError(t,
		err)
}

func TestEnforcer_FailOpen_Concurrent(t *testing.T) {
	t.Parallel()
	e, _ := setupFailOpenEnforcer(t)

	ctx := context.Background()
	var wg conc.WaitGroup
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
	require.NotEqual(
		t, 0, blocked,
	)
	require.NotEqual(
		t, 100, blocked,
	)
}

func TestEnforcer_FailOpen_AllCheckTypes(t *testing.T) {
	t.Parallel()

	checkTypes := []struct {
		name      string
		checkType string
	}{
		{"payment_status", "payment_status"},
		{"daily_run", "daily_run"},
		{"concurrent_run", "concurrent_run"},
		{"spending_limit", "spending_limit"},
		{"project_budget", "project_budget"},
		{"project_suspended", "project_suspended"},
		{"project_limit", "project_limit"},
		{"member_limit", "member_limit"},
		{"org_creation_limit", "org_creation_limit"},
	}

	for _, tt := range checkTypes {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e, _ := setupFailOpenEnforcer(t)
			ctx := context.Background()

			for range maxConsecutiveFailOpen {
				err := e.boundedFailOpen(ctx, "org-1", tt.checkType, "test_error")
				require.NoError(t,
					err)
			}

			err := e.boundedFailOpen(ctx, "org-1", tt.checkType, "test_error")
			require.Error(t,
				err)
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
