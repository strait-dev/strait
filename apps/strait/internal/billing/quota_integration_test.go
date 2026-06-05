//go:build integration

package billing_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckProjectLimit_Integration exercises the project-count gate against
// real Postgres so the LimitError shape, the cache invalidation path on plan
// changes, and the unlimited tier behaviour are all covered end-to-end.
func TestCheckProjectLimit_Integration(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)
	enforcer := billing.NewEnforcer(pgStore, nil, slog.Default())

	orgID := "org-quota-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID, string(domain.
				PlanStarter,
			), "active"))

	enforcer.InvalidateOrgCache(orgID)

	// Under the Starter cap (3 projects): two creates succeed, third pushes
	// us up to the limit but the check runs *before* insertion so it should
	// still allow.
	for range billing.MaxProjectsStarter {
		require.NoError(t, enforcer.
			CheckProjectLimit(
				ctx, orgID,
			))

		createProject(t, ctx, q, orgID, "p"+newID())
	}

	// We now have MaxProjectsStarter projects; a fourth must be rejected
	// with a structured LimitError that carries the canonical fields.
	err := enforcer.CheckProjectLimit(ctx, orgID)
	require.Error(t, err)

	var le *billing.LimitError
	require.True(t, errors.As(
		err, &le),
	)
	assert.Equal(t, "project_limit_reached",

		le.Code,
	)
	assert.Equal(t, int64(billing.
		MaxProjectsStarter,
	), le.Limit,
	)
	assert.Equal(t, int64(billing.
		MaxProjectsStarter,
	), le.CurrentUsage,
	)
	assert.Equal(t, string(domain.
		PlanStarter,
	), le.
		Plan)
	assert.NotEqual(t, "", le.
		UpgradeURL,
	)
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID, string(domain.
				PlanPro,
			), "active"))

	// Upgrade to Pro, invalidate the cached limits, and confirm the same
	// org can now create more projects (10 > 3 existing).

	enforcer.InvalidateOrgCache(orgID)
	require.NoError(t, enforcer.
		CheckProjectLimit(
			ctx, orgID,
		))

}

// TestCheckProjectLimit_UnlimitedTier confirms unlimited tiers (Enterprise,
// represented as MaxProjectsPerOrg = -1) short-circuit the count query so
// extremely large orgs don't pay the lookup cost on every dispatch.
func TestCheckProjectLimit_UnlimitedTier(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	pgStore := billing.NewPgStore(testDB.Pool)
	q := mustQueries(t)
	enforcer := billing.NewEnforcer(pgStore, nil, slog.Default())

	orgID := "org-unlimited-" + newID()
	require.NoError(t, pgStore.
		EnsureOrgSubscription(ctx, orgID))
	require.NoError(t, pgStore.
		UpdateOrgSubscriptionPlan(ctx,
			orgID, string(domain.
				PlanEnterprise,
			), "active"))

	enforcer.InvalidateOrgCache(orgID)

	// Seed well beyond any finite tier and confirm we still pass.
	for range 25 {
		createProject(t, ctx, q, orgID, "p"+newID())
	}
	require.NoError(t, enforcer.
		CheckProjectLimit(
			ctx, orgID,
		))

}
