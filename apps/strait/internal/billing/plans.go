package billing

import "strait/internal/domain"

// OrgPlanLimits defines the complete set of limits and features for a plan tier.
type OrgPlanLimits struct {
	PlanTier                domain.PlanTier
	DisplayName             string
	PriceMonthlyUsd         int   // cents: 1999 = $19.99
	PriceAnnualUsd          int   // cents: 19999 = $199.99
	MaxOrgsPerUser          int   // -1 = unlimited
	MaxProjectsPerOrg       int   // -1 = unlimited
	MaxMembersPerOrg        int   // -1 = unlimited
	MaxRunsPerDay           int64 // -1 = unlimited
	MaxConcurrentRuns       int   // -1 = unlimited
	ComputeCreditMicrousd   int64 // 19990000 = $19.99
	FreeManagedRunsPerMonth int   // free tier only
	FreeManagedPreset       string
	FreeManagedMaxTimeout   int // seconds, free tier only
	RetentionDays           int
	AllowedRegions          []string // nil = all
	MaxAlertRulesPerProj    int      // -1 = unlimited
	MaxWebhookSubsPerProj   int      // -1 = unlimited
	MaxLogDrainsPerOrg      int      // -1 = unlimited
	MaxAIModelCallsPerDay   int      // -1 = unlimited
	AIAssistantBYOK         bool
	HasRBAC                 bool
	RBACLevel               string // "", "basic", "full"
	HasAuditLogs            bool
	HasSSO                  bool
	HasSLA                  bool
	RequiresCreditCard      bool
	OveragePerKRunsMicrousd int64  // cost per 1K runs overage in micro-USD
	AllowsHTTPMode          bool   // whether HTTP execution mode is available
	SupportLevel            string // "community", "email_48h", "priority_24h", "dedicated"
}

// Pricing constants in their respective units.
// These are the canonical values — all plan definitions below reference them.
const (
	// HTTPCostPerRunMicrousd is the per-run cost for HTTP execution mode.
	// 20 micro-USD = $0.00002/run = $20/1M runs.
	HTTPCostPerRunMicrousd int64 = 20

	// Plan prices in cents (USD).
	PriceStarterMonthlyCents = 1999  // $19.99
	PriceStarterAnnualCents  = 19999 // $199.99
	PriceProMonthlyCents     = 4999  // $49.99
	PriceProAnnualCents      = 49999 // $499.99

	// Compute credits in micro-USD, matching subscription price.
	CreditStarterMicrousd int64 = 19_990_000 // $19.99
	CreditProMicrousd     int64 = 49_990_000 // $49.99

	// Daily run limits per plan.
	DailyRunsFree    int64 = 5_000
	DailyRunsStarter int64 = 25_000
	DailyRunsPro     int64 = 100_000

	// Concurrent run limits per plan.
	ConcurrentFree    = 5
	ConcurrentStarter = 25
	ConcurrentPro     = 100

	// Overage cost per 1K runs in micro-USD ($0.20/1K runs).
	OveragePerKRunsMicrousd int64 = 200_000

	// Data retention in days.
	RetentionFree       = 1
	RetentionStarter    = 7
	RetentionPro        = 30
	RetentionEnterprise = 90

	// Organization limits.
	MaxOrgsFree       = 1
	MaxOrgsStarter    = 2
	MaxOrgsPro        = 5
	MaxProjectsFree   = 2
	MaxProjectsStart  = 5
	MaxProjectsPro    = 15
	MaxMembersFree    = 3
	MaxMembersStarter = 10
	MaxMembersPro     = 25

	// Free tier managed execution limits.
	FreeManagedRunsPerMonth = 100
	FreeManagedMaxTimeout   = 10 // seconds

	// Spending limit caps per tier in micro-USD.
	MaxSpendingStarter int64 = 500_000_000   // $500
	MaxSpendingPro     int64 = 2_000_000_000 // $2,000

	// Total available regions (used when AllowedRegions is nil = all).
	TotalRegions = 25
)

// Plans maps plan tiers to their limits.
var Plans = map[domain.PlanTier]OrgPlanLimits{
	domain.PlanFree: {
		PlanTier:                domain.PlanFree,
		DisplayName:             "Free",
		PriceMonthlyUsd:         0,
		PriceAnnualUsd:          0,
		MaxOrgsPerUser:          MaxOrgsFree,
		MaxProjectsPerOrg:       MaxProjectsFree,
		MaxMembersPerOrg:        MaxMembersFree,
		MaxRunsPerDay:           DailyRunsFree,
		MaxConcurrentRuns:       ConcurrentFree,
		ComputeCreditMicrousd:   0,
		FreeManagedRunsPerMonth: FreeManagedRunsPerMonth,
		FreeManagedPreset:       "micro",
		FreeManagedMaxTimeout:   FreeManagedMaxTimeout,
		RetentionDays:           RetentionFree,
		AllowedRegions:          []string{"iad"},
		MaxAlertRulesPerProj:    3,
		MaxWebhookSubsPerProj:   2,
		MaxLogDrainsPerOrg:      0,
		MaxAIModelCallsPerDay:   20,
		AIAssistantBYOK:         false,
		HasRBAC:                 false,
		RBACLevel:               "",
		HasAuditLogs:            false,
		HasSSO:                  false,
		HasSLA:                  false,
		RequiresCreditCard:      false,
		OveragePerKRunsMicrousd: 0,
		AllowsHTTPMode:          false,
		SupportLevel:            "community",
	},
	domain.PlanStarter: {
		PlanTier:                domain.PlanStarter,
		DisplayName:             "Starter",
		PriceMonthlyUsd:         PriceStarterMonthlyCents,
		PriceAnnualUsd:          PriceStarterAnnualCents,
		MaxOrgsPerUser:          MaxOrgsStarter,
		MaxProjectsPerOrg:       MaxProjectsStart,
		MaxMembersPerOrg:        MaxMembersStarter,
		MaxRunsPerDay:           DailyRunsStarter,
		MaxConcurrentRuns:       ConcurrentStarter,
		ComputeCreditMicrousd:   CreditStarterMicrousd,
		FreeManagedRunsPerMonth: 0,
		FreeManagedPreset:       "",
		FreeManagedMaxTimeout:   0,
		RetentionDays:           RetentionStarter,
		AllowedRegions:          []string{"iad", "lax", "lhr", "fra", "nrt", "syd"},
		MaxAlertRulesPerProj:    10,
		MaxWebhookSubsPerProj:   10,
		MaxLogDrainsPerOrg:      1,
		MaxAIModelCallsPerDay:   100,
		AIAssistantBYOK:         false,
		HasRBAC:                 true,
		RBACLevel:               "basic",
		HasAuditLogs:            false,
		HasSSO:                  false,
		HasSLA:                  false,
		RequiresCreditCard:      true,
		OveragePerKRunsMicrousd: OveragePerKRunsMicrousd,
		AllowsHTTPMode:          false,
		SupportLevel:            "email_48h",
	},
	domain.PlanPro: {
		PlanTier:                domain.PlanPro,
		DisplayName:             "Pro",
		PriceMonthlyUsd:         PriceProMonthlyCents,
		PriceAnnualUsd:          PriceProAnnualCents,
		MaxOrgsPerUser:          MaxOrgsPro,
		MaxProjectsPerOrg:       MaxProjectsPro,
		MaxMembersPerOrg:        MaxMembersPro,
		MaxRunsPerDay:           DailyRunsPro,
		MaxConcurrentRuns:       ConcurrentPro,
		ComputeCreditMicrousd:   CreditProMicrousd,
		FreeManagedRunsPerMonth: 0,
		FreeManagedPreset:       "",
		FreeManagedMaxTimeout:   0,
		RetentionDays:           RetentionPro,
		AllowedRegions:          nil,
		MaxAlertRulesPerProj:    50,
		MaxWebhookSubsPerProj:   50,
		MaxLogDrainsPerOrg:      5,
		MaxAIModelCallsPerDay:   500,
		AIAssistantBYOK:         true,
		HasRBAC:                 true,
		RBACLevel:               "full",
		HasAuditLogs:            true,
		HasSSO:                  false,
		HasSLA:                  false,
		RequiresCreditCard:      true,
		OveragePerKRunsMicrousd: OveragePerKRunsMicrousd,
		AllowsHTTPMode:          true,
		SupportLevel:            "priority_24h",
	},
	domain.PlanEnterprise: {
		PlanTier:                domain.PlanEnterprise,
		DisplayName:             "Enterprise",
		PriceMonthlyUsd:         0,
		PriceAnnualUsd:          0,
		MaxOrgsPerUser:          -1,
		MaxProjectsPerOrg:       -1,
		MaxMembersPerOrg:        -1,
		MaxRunsPerDay:           -1,
		MaxConcurrentRuns:       -1,
		ComputeCreditMicrousd:   0,
		FreeManagedRunsPerMonth: 0,
		FreeManagedPreset:       "",
		FreeManagedMaxTimeout:   0,
		RetentionDays:           RetentionEnterprise,
		AllowedRegions:          nil,
		MaxAlertRulesPerProj:    -1,
		MaxWebhookSubsPerProj:   -1,
		MaxLogDrainsPerOrg:      -1,
		MaxAIModelCallsPerDay:   -1,
		AIAssistantBYOK:         true,
		HasRBAC:                 true,
		RBACLevel:               "full",
		HasAuditLogs:            true,
		HasSSO:                  true,
		HasSLA:                  true,
		RequiresCreditCard:      false,
		OveragePerKRunsMicrousd: 0,
		AllowsHTTPMode:          true,
		SupportLevel:            "dedicated",
	},
}

// IsDowngrade returns true if moving from oldTier to newTier reduces ANY limit.
// Used to determine whether a plan change should be deferred to period end.
func IsDowngrade(oldTier, newTier domain.PlanTier) bool {
	if oldTier == newTier {
		return false
	}
	oldLimits := GetPlanLimits(oldTier)
	newLimits := GetPlanLimits(newTier)

	// Compare all numeric limits. A decrease in ANY field means downgrade.
	// -1 means unlimited, so going from -1 to any positive number is a downgrade.
	less := func(oldVal, newVal int64) bool {
		if oldVal == -1 && newVal != -1 {
			return true // unlimited -> limited
		}
		if newVal == -1 {
			return false // anything -> unlimited is not a downgrade
		}
		return newVal < oldVal
	}
	lessInt := func(oldVal, newVal int) bool {
		return less(int64(oldVal), int64(newVal))
	}

	return less(oldLimits.MaxRunsPerDay, newLimits.MaxRunsPerDay) ||
		lessInt(oldLimits.MaxConcurrentRuns, newLimits.MaxConcurrentRuns) ||
		lessInt(oldLimits.MaxProjectsPerOrg, newLimits.MaxProjectsPerOrg) ||
		lessInt(oldLimits.MaxMembersPerOrg, newLimits.MaxMembersPerOrg) ||
		lessInt(oldLimits.MaxOrgsPerUser, newLimits.MaxOrgsPerUser) ||
		less(oldLimits.ComputeCreditMicrousd, newLimits.ComputeCreditMicrousd) ||
		lessInt(oldLimits.RetentionDays, newLimits.RetentionDays) ||
		lessInt(oldLimits.MaxAlertRulesPerProj, newLimits.MaxAlertRulesPerProj) ||
		lessInt(oldLimits.MaxWebhookSubsPerProj, newLimits.MaxWebhookSubsPerProj) ||
		lessInt(oldLimits.MaxLogDrainsPerOrg, newLimits.MaxLogDrainsPerOrg) ||
		lessInt(oldLimits.MaxAIModelCallsPerDay, newLimits.MaxAIModelCallsPerDay)
}

// GetPlanLimits returns the plan limits for the given tier.
// Returns free plan limits if the tier is unknown.
func GetPlanLimits(tier domain.PlanTier) OrgPlanLimits {
	if limits, ok := Plans[tier]; ok {
		return limits
	}
	return Plans[domain.PlanFree]
}
