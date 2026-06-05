package billing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"strait/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetOrgRetentionDays_EmptyOrgID(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "")
	require.Error(t,
		err)
	assert.Equal(t, 0,
		days)
}

func TestGetOrgRetentionDays_SubscriptionNotFound(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-no-sub")
	require.ErrorIs(t, err, ErrSubscriptionNotFound)
	assert.Equal(t, 0,
		days)
}

func TestGetOrgRetentionDays_StoreErrorDoesNotFallbackToShorterRetention(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("database unavailable")
		},
	}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-pro")
	require.Error(t,
		err)
	assert.Equal(t, 0,
		days)
}

func TestDeepSecGetOrgRetentionDays_AddsActiveHistoryAddons(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-scale": {
				OrgID:    "org-scale",
				PlanTier: "scale",
				Status:   "active",
			},
		},
		activeAddons: []Addon{
			{OrgID: "org-scale", AddonType: AddonHistory30d, Quantity: 2, Active: true},
			{OrgID: "org-scale", AddonType: AddonHistory30d, Quantity: 10, Active: false},
		},
	}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-scale")
	require.NoError(t,
		err)

	want := GetPlanLimits(domain.PlanScale).RetentionDays + 60
	assert.Equal(t, want,
		days)
}

func TestDeepSecGetOrgRetentionDays_UnlimitedRetentionRemainsUnlimited(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-enterprise": {
				OrgID:    "org-enterprise",
				PlanTier: "enterprise",
				Status:   "active",
			},
		},
		activeAddons: []Addon{
			{OrgID: "org-enterprise", AddonType: AddonHistory30d, Quantity: 10, Active: true},
		},
	}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-enterprise")
	require.NoError(t,
		err)
	assert.Equal(t, -1, days)
}

func TestDeepSecGetOrgRetentionDays_AddonLookupErrorSkipsRetention(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-pro": {OrgID: "org-pro", PlanTier: "pro", Status: "active"},
		},
		listActiveAddonsErr: errors.New("addon store down"),
	}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-pro")
	require.Error(t,
		err)
	assert.Equal(t, 0,
		days)
}

func TestGetOrgRetentionDays_ProPlan(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"org-pro": {OrgID: "org-pro", PlanTier: "pro", Status: "active"},
		},
	}
	resolver := NewPlanRetentionResolver(store)

	days, err := resolver.GetOrgRetentionDays(context.Background(), "org-pro")
	require.NoError(t,
		err)

	want := GetPlanLimits(domain.PlanPro).RetentionDays
	assert.Equal(t, want,
		days)
}

func TestListAllSubscribedOrgIDs(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	resolver := NewPlanRetentionResolver(store)

	ids, err := resolver.ListAllSubscribedOrgIDs(context.Background())
	require.NoError(t,
		err)
	assert.Nil(t, ids)
}

func TestListAllSubscribedOrgIDsSQL_HasNoFixedRetentionSweepCap(t *testing.T) {
	t.Parallel()

	sql := strings.ToUpper(listAllSubscribedOrgIDsSQL())
	require.NotContains(t,
		sql, "LIMIT")
	require.Contains(t, sql, "ORDER BY ORG_ID")
}
