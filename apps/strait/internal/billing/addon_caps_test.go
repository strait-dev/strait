package billing

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"strait/internal/domain"
)

func TestAddonCap_UnderLimit_Allows(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			"550e8400-e29b-41d4-a716-446655440000": {OrgID: "550e8400-e29b-41d4-a716-446655440000", PlanTier: string(domain.PlanStarter), Status: "active"},
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	mapping := NewStripeMappingFromOptions(
		WithStarterPrices("starter-id", ""),
		WithProPrices("pro-id", ""),
		WithAddonPrice("addon-conc", AddonConcurrency100),
	)

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), enforcer, nil, WithDevBypassSignatureCheck(),
		WithEdition("community"))

	// Existing count: 0, cap: whatever the plan allows -- should allow.
	sub := testSubscriptionData{
		ID:         "sub_addon_1",
		ProductID:  "addon-conc",
		CustomerID: "cust_1",
		Status:     "active",
		Metadata:   map[string]string{"org_id": "550e8400-e29b-41d4-a716-446655440000"},
	}
	err := handler.handleAddonSubscriptionCreated(context.Background(), sub.ToStripe(), AddonConcurrency100, "")
	require.NoError(t,
		err)

}

func TestAddonCap_PlanLimitLookupErrorFailsClosed(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	orgID := "550e8400-e29b-41d4-a716-446655440030"
	store := &mockBillingStore{
		getOrgSubscriptionFn: func(context.Context, string) (*OrgSubscription, error) {
			return nil, errors.New("subscription store unavailable")
		},
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	handler := NewWebhookHandler(store, NewStripeMappingFromOptions(), "", slog.Default(), enforcer, nil,
		WithDevBypassSignatureCheck(), WithEdition("community"))

	sub := testSubscriptionData{
		ID:         "sub_addon_limit_lookup_failure",
		ProductID:  "addon-conc",
		CustomerID: "cust_limit_lookup_failure",
		Status:     "active",
		Metadata:   map[string]string{"org_id": orgID},
	}
	err := handler.handleAddonSubscriptionCreated(context.Background(), sub.ToStripe(), AddonConcurrency100, "")
	require.Error(t,
		err)
	assert.Contains(t,
		err.Error(), "get org plan limits for addon subscription",
	)
	require.Nil(t, store.
		lastAddonCreated,
	)

}

func TestAddonCap_CountErrorFailsClosed(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	orgID := "550e8400-e29b-41d4-a716-446655440031"
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {OrgID: orgID, PlanTier: string(domain.PlanScale), Status: "active"},
		},
		countActiveAddonsErr: errors.New("addon count unavailable"),
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	handler := NewWebhookHandler(store, NewStripeMappingFromOptions(), "", slog.Default(), enforcer, nil,
		WithDevBypassSignatureCheck(), WithEdition("community"))

	sub := testSubscriptionData{
		ID:         "sub_addon_count_failure",
		ProductID:  "addon-history",
		CustomerID: "cust_count_failure",
		Status:     "active",
		Metadata:   map[string]string{"org_id": orgID},
	}
	err := handler.handleAddonSubscriptionCreated(context.Background(), sub.ToStripe(), AddonHistory30d, "")
	require.Error(t,
		err)
	assert.Contains(t,
		err.Error(), "count active addons for addon subscription",
	)
	require.Nil(t, store.
		lastAddonCreated,
	)

}

func TestAddonCap_CapExceededDoesNotCreateActiveAddon(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	orgID := "550e8400-e29b-41d4-a716-446655440032"
	cap := GetPlanLimits(domain.PlanScale).MaxAddonPacks[AddonHistory30d]
	activeAddons := make([]Addon, 0, cap)
	for i := range cap {
		activeAddons = append(activeAddons, Addon{
			ID:        "addon-history-existing-" + string(rune('a'+i)),
			OrgID:     orgID,
			AddonType: AddonHistory30d,
			Active:    true,
			Quantity:  1,
		})
	}
	store := &mockBillingStore{
		subscriptions: map[string]*OrgSubscription{
			orgID: {OrgID: orgID, PlanTier: string(domain.PlanScale), Status: "active"},
		},
		activeAddons: activeAddons,
	}
	enforcer := NewEnforcer(store, rdb, slog.Default())
	handler := NewWebhookHandler(store, NewStripeMappingFromOptions(), "", slog.Default(), enforcer, nil,
		WithDevBypassSignatureCheck(), WithEdition("community"))

	sub := testSubscriptionData{
		ID:         "sub_addon_cap_exceeded",
		ProductID:  "addon-history",
		CustomerID: "cust_cap_exceeded",
		Status:     "active",
		Metadata:   map[string]string{"org_id": orgID},
	}
	err := handler.handleAddonSubscriptionCreated(context.Background(), sub.ToStripe(), AddonHistory30d, "")
	require.NoError(t,
		err)
	require.Nil(t, store.
		lastAddonCreated,
	)

}

func TestAddonCap_EnforcerNil_Allows(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	mapping := NewStripeMappingFromOptions(
		WithStarterPrices("starter-id", ""),
		WithProPrices("pro-id", ""),
	)

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil, WithDevBypassSignatureCheck())

	sub := testSubscriptionData{
		ID:         "sub_addon_2",
		ProductID:  "addon-conc",
		CustomerID: "cust_1",
		Status:     "active",
		Metadata:   map[string]string{"org_id": "550e8400-e29b-41d4-a716-446655440000"},
	}
	err := handler.handleAddonSubscriptionCreated(context.Background(), sub.ToStripe(), AddonConcurrency100, "")
	require.NoError(t,
		err)
	require.NotNil(t,
		store.lastAddonCreated,
	)

}

func TestAddonCap_EnforcerNil_RoadmapAddonDoesNotCreateActiveAddon(t *testing.T) {
	t.Parallel()
	store := &mockBillingStore{}
	handler := NewWebhookHandler(store, NewStripeMappingFromOptions(), "", slog.Default(), nil, nil,
		WithDevBypassSignatureCheck())

	sub := testSubscriptionData{
		ID:         "sub_roadmap_addon",
		ProductID:  "addon-roadmap",
		CustomerID: "cust_roadmap",
		Status:     "active",
		Metadata:   map[string]string{"org_id": "550e8400-e29b-41d4-a716-446655440000"},
	}
	err := handler.handleAddonSubscriptionCreated(context.Background(), sub.ToStripe(), AddonComplianceArchive, "")
	require.NoError(t,
		err)
	require.Nil(t, store.
		lastAddonCreated,
	)

}

func TestAddonCap_DisallowedAddonOnPlan_DoesNotCreateActiveAddon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		planTier  domain.PlanTier
		addonType AddonType
	}{
		{
			name:      "roadmap_compliance_archive_on_pro",
			planTier:  domain.PlanPro,
			addonType: AddonComplianceArchive,
		},
		{
			name:      "environment_pack_on_business",
			planTier:  domain.PlanBusiness,
			addonType: AddonEnvironments5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mr := miniredis.RunT(t)
			rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			orgID := "550e8400-e29b-41d4-a716-446655440010"
			store := &mockBillingStore{
				subscriptions: map[string]*OrgSubscription{
					orgID: {OrgID: orgID, PlanTier: string(tt.planTier), Status: "active"},
				},
			}
			enforcer := NewEnforcer(store, rdb, slog.Default())
			handler := NewWebhookHandler(store, NewStripeMappingFromOptions(), "", slog.Default(), enforcer, nil,
				WithDevBypassSignatureCheck(), WithEdition("community"))

			sub := testSubscriptionData{
				ID:         "sub_disallowed_" + string(tt.addonType),
				ProductID:  "addon-price-id",
				CustomerID: "cust_disallowed",
				Status:     "active",
				Metadata:   map[string]string{"org_id": orgID},
			}
			err := handler.handleAddonSubscriptionCreated(context.Background(), sub.ToStripe(), tt.addonType, "")
			require.NoError(t,
				err)
			require.Nil(t, store.
				lastAddonCreated,
			)

		})
	}
}
