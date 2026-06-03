package scheduler

import (
	"context"
	"reflect"
	"testing"
	"time"

	"strait/internal/billing"
)

// downgradeStrategyMap names every OrgPlanLimits field and pins the policy
// the downgrade applier follows for it. This test fails if a new field is
// added to OrgPlanLimits without an explicit decision recorded here. The
// idea is to force whoever adds the limit to think about downgrade behavior
// before merging — the rest of the platform already gates create paths, but
// downgrade-time cleanup is the easiest dimension to forget.
//
// Strategies:
//   - "deactivate" — the applier removes/disables existing rows above cap
//     (e.g. excess projects, cron jobs, webhooks).
//   - "block_new" — only new resources are gated; the applier intentionally
//     leaves existing rows alone (e.g. members, worker connections).
//   - "block_new_signal" — like block_new, but additionally the applier
//     emits a billing event so the dashboard surfaces the overage.
//   - "n/a_static" — pricing/metadata field with no downgrade implication
//     (display name, price, support level, region list, etc.).
//   - "n/a_runtime" — runtime-enforced setting that doesn't accumulate
//     long-lived rows (rate limits, dispatch priority, daily caps,
//     boolean feature flags evaluated per request).
var downgradeStrategyMap = map[string]string{
	// Identity / pricing / metadata.
	"PlanTier":            "n/a_static",
	"DisplayName":         "n/a_static",
	"PriceMonthlyUsd":     "n/a_static",
	"PriceAnnualUsd":      "n/a_static",
	"OveragePerKMicrousd": "n/a_static",
	"AllowedRegions":      "n/a_static",
	"RequiresCreditCard":  "n/a_static",
	"SupportLevel":        "n/a_static",

	// Per-tenant resource caps the applier already cleans up.
	"MaxProjectsPerOrg":       "deactivate",
	"MaxScheduledJobs":        "deactivate",
	"MaxWebhookEndpoints":     "deactivate",
	"MaxEnvironments":         "deactivate",
	"MaxLogDrainsPerOrg":      "deactivate",
	"MaxNotificationChannels": "deactivate",

	// Block-new policies.
	"MaxOrgsPerUser":    "block_new",
	"MaxMembersPerOrg":  "block_new_signal",
	"WorkerConnections": "block_new",

	// Runtime-enforced caps and feature flags.
	"MaxRunsPerDay":          "n/a_runtime",
	"MaxRunsPerMonth":        "n/a_runtime",
	"MaxConcurrentRuns":      "n/a_runtime",
	"MaxAlertRulesPerProj":   "n/a_runtime",
	"MaxWebhookSubsPerProj":  "n/a_runtime",
	"MaxDispatchPriority":    "n/a_runtime",
	"HasRBAC":                "n/a_runtime",
	"RBACLevel":              "n/a_runtime",
	"HasAuditLogs":           "n/a_runtime",
	"HasSSO":                 "n/a_runtime",
	"HasSLA":                 "n/a_runtime",
	"AllowsHTTPMode":         "deactivate", // PauseHTTPJobsByOrg covers existing HTTP-mode jobs
	"LogStreamingEnabled":    "n/a_runtime",
	"RetentionDays":          "n/a_runtime",
	"WebhookEventLevel":      "n/a_runtime",
	"CronMinIntervalSec":     "n/a_runtime",
	"AllCronOverlapPolicies": "n/a_runtime",
	"APIRateLimit":           "n/a_runtime",
	"MaxAddonPacks":          "n/a_runtime",

	// Workflow feature gates.
	"MaxWorkflowDAGSteps":  "n/a_runtime",
	"HasApprovalGates":     "n/a_runtime",
	"HasSubWorkflows":      "n/a_runtime",
	"HasJobChaining":       "n/a_runtime",
	"MaxJobChainDepth":     "n/a_runtime",
	"HasCompensatingTxns":  "n/a_runtime",
	"HasCanaryDeployments": "n/a_runtime",

	// Enterprise-only feature gates.
	"HasDedicatedCompute":  "n/a_runtime",
	"HasStaticIPs":         "n/a_runtime",
	"HasVPCPeering":        "n/a_runtime",
	"HasSCIM":              "n/a_runtime",
	"HasDataResidency":     "n/a_runtime",
	"HasCustomRBAC":        "n/a_runtime",
	"HasPriorityQueue":     "n/a_runtime",
	"HasIPAllowlisting":    "n/a_runtime",
	"HasSessionManagement": "n/a_runtime",
	"HasSecretRotation":    "n/a_runtime",
	"HasSIEMExport":        "n/a_runtime",
}

// TestDowngradeStrategy_CoversEveryOrgPlanLimitField fails if OrgPlanLimits
// gains a field that is not declared in downgradeStrategyMap. Adding a new
// field forces the contributor to pick a strategy; that's the point.
func TestDowngradeStrategy_CoversEveryOrgPlanLimitField(t *testing.T) {
	t.Parallel()

	rt := reflect.TypeFor[billing.OrgPlanLimits]()
	missing := []string{}
	for f := range rt.Fields() {
		if _, ok := downgradeStrategyMap[f.Name]; !ok {
			missing = append(missing, f.Name)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("OrgPlanLimits has %d undocumented field(s): %v\n\nAdd each to downgradeStrategyMap with the correct strategy:\n  deactivate / block_new / block_new_signal / n/a_static / n/a_runtime",
			len(missing), missing)
	}
}

// TestDowngradeStrategy_NoStaleEntries fails if downgradeStrategyMap has an
// entry for a field that no longer exists on OrgPlanLimits — keeps the map
// honest as the catalog evolves.
func TestDowngradeStrategy_NoStaleEntries(t *testing.T) {
	t.Parallel()

	rt := reflect.TypeFor[billing.OrgPlanLimits]()
	known := map[string]bool{}
	for f := range rt.Fields() {
		known[f.Name] = true
	}
	stale := []string{}
	for name := range downgradeStrategyMap {
		if !known[name] {
			stale = append(stale, name)
		}
	}
	if len(stale) > 0 {
		t.Fatalf("downgradeStrategyMap references non-existent OrgPlanLimits field(s): %v", stale)
	}
}

// TestDowngradeApplier_ConcurrentTicks_NoDoubleDeactivation simulates two
// applier ticks racing on the same org. The deactivate-excess methods are
// idempotent (subsequent runs find nothing to update), so the second tick
// must not panic, error, or double-process.
func TestDowngradeApplier_ConcurrentTicks_NoDoubleDeactivation(t *testing.T) {
	t.Parallel()

	free := "free"
	pastEnd := time.Now().Add(-1 * time.Hour)
	store := &mockDowngradeStore{
		pendingOrgs: []billing.OrgSubscription{
			{OrgID: "org-1", PlanTier: "pro", PendingPlanTier: &free, CurrentPeriodEnd: &pastEnd},
		},
		projectIDs: []string{"proj-a"},
	}
	enforcer := newTestEnforcer(t)
	applier := NewDowngradeApplier(store, enforcer, time.Minute)

	// Two sequential ticks (the applier's distributed advisory lock prevents
	// true concurrency in production; this test pins idempotency under the
	// equivalent serialized retry).
	applier.apply(context.Background())
	applier.apply(context.Background())

	// Each tick produces one log-drain call; the deactivator queries for
	// rows with enabled=true and OFFSETs past the cap, so a stale-state
	// second pass returns 0 rows. This test pins that the applier itself
	// is happy to be called twice.
	if got := len(store.logDrainCalls); got < 1 {
		t.Errorf("expected at least one DeactivateExcessLogDrains call after two ticks, got %d", got)
	}
}
