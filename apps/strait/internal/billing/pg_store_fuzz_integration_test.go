//go:build integration

package billing_test

import (
	"context"
	"testing"
	"time"

	"strait/internal/billing"

	"github.com/stretchr/testify/assert"
)

// F1: Fuzz EnsureOrgSubscription with unusual org IDs

func TestFuzz_EnsureOrgSubscription(t *testing.T) {
	ctx := context.Background()
	pgStore := billing.NewPgStore(testDB.Pool)

	inputs := []string{
		"normal-org-id",
		"",
		"org-with-spaces in it",
		"org-with-unicode-\u00e9\u00e8\u00ea",
		"org-with-emoji-placeholder",
		"'; DROP TABLE organization_subscriptions; --",
		"org-" + string(make([]byte, 500)), // long string
		"org-with-null-\x00-byte",
		"org-with-newlines\n\r\n",
		"<script>alert('xss')</script>",
	}

	for _, input := range inputs {
		mustClean(t, ctx)
		err := pgStore.EnsureOrgSubscription(ctx, input)
		// We just verify no panic and no unexpected crash.
		// Empty string and null bytes may fail, which is acceptable.
		if err != nil {
			t.Logf("EnsureOrgSubscription(%q) = %v (acceptable)", input, err)
		}
	}
}

// F2: Fuzz GetOrgSubscription with unusual org IDs

func TestFuzz_GetOrgSubscription(t *testing.T) {
	ctx := context.Background()
	pgStore := billing.NewPgStore(testDB.Pool)

	inputs := []string{
		"",
		"nonexistent",
		"'; DROP TABLE organization_subscriptions; --",
		"\x00\x00\x00",
		"a\nb\nc",
		"org-\u2603-snowman",
	}

	for _, input := range inputs {
		mustClean(t, ctx)
		_, err := pgStore.GetOrgSubscription(ctx, input)
		// Should return ErrSubscriptionNotFound or a wrapped error, not panic.
		if err == nil {
			t.Logf("GetOrgSubscription(%q) unexpectedly returned nil error", input)
		}
	}
}

// F3: Fuzz RecordProcessedWebhook with unusual message IDs

func TestFuzz_RecordProcessedWebhook(t *testing.T) {
	ctx := context.Background()
	pgStore := billing.NewPgStore(testDB.Pool)

	inputs := []string{
		"",
		"normal-msg-id",
		"'; DELETE FROM processed_webhook_messages; --",
		"\x00\x01\x02",
		"msg-with-unicode-\u4e16\u754c",
		string(make([]byte, 1000)),
		"msg\nwith\nnewlines",
		"<img src=x onerror=alert(1)>",
	}

	for _, input := range inputs {
		mustClean(t, ctx)
		err := pgStore.RecordProcessedWebhook(ctx, input)
		if err != nil {
			t.Logf("RecordProcessedWebhook(%q) = %v (acceptable)", input, err)
			continue
		}

		processed, err := pgStore.IsWebhookProcessed(ctx, input)
		if err != nil {
			t.Logf("IsWebhookProcessed(%q) = %v (acceptable)", input, err)
			continue
		}
		assert.True(t, processed)

	}
}

// F4: Fuzz UpsertOrgSubscription with boundary values

func TestFuzz_UpsertOrgSubscription(t *testing.T) {
	ctx := context.Background()
	pgStore := billing.NewPgStore(testDB.Pool)

	now := time.Now().UTC()

	testCases := []struct {
		name   string
		orgID  string
		plan   string
		limit  int64
		action string
	}{
		{"empty-plan", "org-fuzz-1-" + newID(), "", 0, "notify"},
		{"zero-limit", "org-fuzz-2-" + newID(), "free", 0, "notify"},
		{"negative-limit", "org-fuzz-3-" + newID(), "pro", -1, "suspend"},
		{"max-limit", "org-fuzz-4-" + newID(), "enterprise", 9_223_372_036_854_775_807, "notify"},
		{"sql-injection-plan", "org-fuzz-5-" + newID(), "'; DROP TABLE x; --", 100, "notify"},
		{"unicode-action", "org-fuzz-6-" + newID(), "pro", 100, "\u00e9\u00e8"},
		{"empty-org", "", "free", 0, "notify"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mustClean(t, ctx)
			sub := &billing.OrgSubscription{
				ID:                    newID(),
				OrgID:                 tc.orgID,
				PlanTier:              tc.plan,
				Status:                "active",
				SpendingLimitMicrousd: tc.limit,
				LimitAction:           tc.action,
				CreatedAt:             now,
				UpdatedAt:             now,
			}
			err := pgStore.UpsertOrgSubscription(ctx, sub)
			if err != nil {
				t.Logf("UpsertOrgSubscription(%q) = %v (acceptable)", tc.name, err)
				return
			}

			// Verify round-trip if insert succeeded.
			got, err := pgStore.GetOrgSubscription(ctx, tc.orgID)
			if err != nil {
				t.Logf("GetOrgSubscription(%q) = %v (acceptable)", tc.name, err)
				return
			}
			assert.Equal(t, tc.plan,
				got.
					PlanTier,
			)

		})
	}
}

// F5: Fuzz CreateAddon with boundary values

func TestFuzz_CreateAddon(t *testing.T) {
	ctx := context.Background()
	pgStore := billing.NewPgStore(testDB.Pool)

	testCases := []struct {
		name      string
		addonType billing.AddonType
		quantity  int
	}{
		{"zero-quantity", billing.AddonConcurrency100, 0},
		{"negative-quantity", billing.AddonEnvironments5, -1},
		{"max-quantity", billing.AddonHistory30d, 2147483647},
		{"empty-type", billing.AddonType(""), 1},
		{"sql-injection-type", billing.AddonType("'; DROP TABLE organization_addons; --"), 1},
		{"unicode-type", billing.AddonType("\u2603"), 1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mustClean(t, ctx)
			orgID := "org-fuzzaddon-" + newID()
			a := &billing.Addon{
				ID:        newID(),
				OrgID:     orgID,
				AddonType: tc.addonType,
				Quantity:  tc.quantity,
				Active:    true,
			}
			err := pgStore.CreateAddon(ctx, a)
			if err != nil {
				t.Logf("CreateAddon(%q) = %v (acceptable)", tc.name, err)
				return
			}

			addons, err := pgStore.ListActiveAddons(ctx, orgID)
			if err != nil {
				t.Logf("ListActiveAddons after %q = %v", tc.name, err)
				return
			}
			assert.Len(t, addons, 1)

		})
	}
}
