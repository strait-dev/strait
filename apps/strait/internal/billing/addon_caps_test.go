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
		WithAddonPrice("addon-conc", AddonConcurrentRuns),
	)

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), enforcer, nil,
		WithEdition("community"))

	// Existing count: 0, cap: whatever the plan allows -- should allow.
	sub := testSubscriptionData{
		ID:         "sub_addon_1",
		ProductID:  "addon-conc",
		CustomerID: "cust_1",
		Status:     "active",
		Metadata:   map[string]string{"org_id": "550e8400-e29b-41d4-a716-446655440000"},
	}
	err := handler.handleAddonSubscriptionCreated(context.Background(), sub.ToStripe(), AddonConcurrentRuns)
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

	handler := NewWebhookHandler(store, mapping, "", slog.Default(), nil, nil)

	sub := testSubscriptionData{
		ID:         "sub_addon_2",
		ProductID:  "addon-conc",
		CustomerID: "cust_1",
		Status:     "active",
		Metadata:   map[string]string{"org_id": "550e8400-e29b-41d4-a716-446655440000"},
	}
	err := handler.handleAddonSubscriptionCreated(context.Background(), sub.ToStripe(), AddonConcurrentRuns)
	if err != nil {
		t.Fatalf("expected addon to be allowed without enforcer, got: %v", err)
	}
}
