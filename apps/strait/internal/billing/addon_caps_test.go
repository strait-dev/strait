package billing

import (
	"context"
	"log/slog"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

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
	if err != nil {
		t.Fatalf("expected addon to be allowed, got: %v", err)
	}
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
	if err != nil {
		t.Fatalf("expected addon to be allowed without enforcer, got: %v", err)
	}
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
			if err != nil {
				t.Fatalf("expected disallowed addon webhook to be ignored without error, got: %v", err)
			}
			if store.lastAddonCreated != nil {
				t.Fatalf("disallowed addon created active row: %#v", store.lastAddonCreated)
			}
		})
	}
}
