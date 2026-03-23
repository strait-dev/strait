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
	SupportLevel            string // "community", "email_48h", "priority_24h", "dedicated"
}

// Plans maps plan tiers to their limits.
var Plans = map[domain.PlanTier]OrgPlanLimits{
	domain.PlanFree: {
		PlanTier:                domain.PlanFree,
		DisplayName:             "Free",
		PriceMonthlyUsd:         0,
		PriceAnnualUsd:          0,
		MaxOrgsPerUser:          1,
		MaxProjectsPerOrg:       2,
		MaxMembersPerOrg:        3,
		MaxRunsPerDay:           5000,
		MaxConcurrentRuns:       5,
		ComputeCreditMicrousd:   0,
		FreeManagedRunsPerMonth: 100,
		FreeManagedPreset:       "micro",
		FreeManagedMaxTimeout:   10,
		RetentionDays:           1,
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
		SupportLevel:            "community",
	},
	domain.PlanStarter: {
		PlanTier:                domain.PlanStarter,
		DisplayName:             "Starter",
		PriceMonthlyUsd:         1999,
		PriceAnnualUsd:          19999,
		MaxOrgsPerUser:          2,
		MaxProjectsPerOrg:       5,
		MaxMembersPerOrg:        10,
		MaxRunsPerDay:           25000,
		MaxConcurrentRuns:       25,
		ComputeCreditMicrousd:   19990000,
		FreeManagedRunsPerMonth: 0,
		FreeManagedPreset:       "",
		FreeManagedMaxTimeout:   0,
		RetentionDays:           7,
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
		OveragePerKRunsMicrousd: 200000,
		SupportLevel:            "email_48h",
	},
	domain.PlanPro: {
		PlanTier:                domain.PlanPro,
		DisplayName:             "Pro",
		PriceMonthlyUsd:         4999,
		PriceAnnualUsd:          49999,
		MaxOrgsPerUser:          5,
		MaxProjectsPerOrg:       15,
		MaxMembersPerOrg:        25,
		MaxRunsPerDay:           100000,
		MaxConcurrentRuns:       100,
		ComputeCreditMicrousd:   49990000,
		FreeManagedRunsPerMonth: 0,
		FreeManagedPreset:       "",
		FreeManagedMaxTimeout:   0,
		RetentionDays:           30,
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
		OveragePerKRunsMicrousd: 200000,
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
		RetentionDays:           90,
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
