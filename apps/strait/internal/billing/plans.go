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
	MaxRunsPerDay           int64 // -1 = unlimited (legacy daily cap; see MaxRunsPerMonth)
	MaxRunsPerMonth         int   // -1 = unlimited; monthly cap for orchestration billing
	OveragePerKMicrousd     int64 // per-1K-run overage in micro-USD (new name); alias for OveragePerKRunsMicrousd
	MaxConcurrentRuns       int   // -1 = unlimited
	RetentionDays           int
	AllowedRegions          []string // nil = all
	MaxAlertRulesPerProj    int      // -1 = unlimited, 0 = none
	MaxWebhookSubsPerProj   int      // -1 = unlimited, 0 = none
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
	LogStreamingEnabled     bool   // whether log streaming is available
	MaxDispatchPriority     int    // max enqueue priority value; 0 = default only, -1 = unlimited
	WorkerConnections       int    // max registered worker connections per org; -1 = unlimited
	SupportLevel            string // "community", "email_72h", "priority_24h", "priority_slack_8h", "dedicated"

	// Workflow feature gates.
	MaxWorkflowDAGSteps  int  // max steps in a workflow DAG; -1 = unlimited
	HasApprovalGates     bool // workflow approval gates (Pro+)
	HasSubWorkflows      bool // sub-workflow support (Pro+)
	HasJobChaining       bool // job chaining support (Pro+)
	MaxJobChainDepth     int  // max chain depth; 0 = disabled, -1 = unlimited
	HasCompensatingTxns  bool // compensating transactions / saga pattern (Pro+)
	HasCanaryDeployments bool // canary deployment support (Scale+)

	// Enterprise-only feature gates.
	HasDedicatedCompute  bool // isolated Fly org for workloads
	HasStaticIPs         bool // fixed egress IP addresses
	HasVPCPeering        bool // private network access
	HasSCIM              bool // directory sync (user provisioning)
	HasDataResidency     bool // regional data isolation
	HasCustomRBAC        bool // org-level role definitions beyond standard
	HasPriorityQueue     bool // enterprise jobs dequeued first
	HasIPAllowlisting    bool // restrict API access to known CIDRs
	HasSessionManagement bool // view/revoke OIDC sessions, bulk key revocation
	HasSecretRotation    bool // zero-downtime secret rotation with grace period
	HasSIEMExport        bool // forward audit logs to external SIEM

	// Resource limits.
	MaxScheduledJobs       int               // max cron schedules; -1 = unlimited
	AllCronOverlapPolicies bool              // false = "allow" only; true = all policies
	MaxEnvironments        int               // max environments per project
	MaxWebhookEndpoints    int               // max webhook endpoints; -1 = unlimited, 0 = none
	WebhookEventLevel      string            // "none", "basic", "all", "all_custom"
	APIRateLimit           int               // requests per minute; -1 = unlimited
	MaxAddonPacks          map[AddonType]int `json:"max_addon_packs,omitempty"` // max packs per addon type; -1 = unlimited
}

// Pricing constants in their respective units.
// These are the canonical values -- all plan definitions below reference them.
const (
	// HTTPCostPerRunMicrousd is the per-run cost for HTTP execution mode.
	// 20 micro-USD = $0.00002/run = $20/1M runs.
	HTTPCostPerRunMicrousd int64 = 20

	// WorkerCostPerRunMicrousd is the per-run cost for Worker execution mode.
	// Flat rate matching HTTP dispatch.
	WorkerCostPerRunMicrousd int64 = 20

	// WebhookDeliveryCostPerRunMicrousd is the per-successful-delivery cost for
	// outbound webhook deliveries. Billed once on eventual success; failed
	// deliveries that never succeed are not billed.
	WebhookDeliveryCostPerRunMicrousd int64 = 20

	// Plan prices in cents (USD). Annual = monthly * 12 * 0.80 (20% off).
	PriceStarterMonthlyCents  = 1_900   // $19
	PriceStarterAnnualCents   = 18_240  // $182.40
	PriceProMonthlyCents      = 9_900   // $99
	PriceProAnnualCents       = 95_040  // $950.40
	PriceScaleMonthlyCents    = 29_900  // $299
	PriceScaleAnnualCents     = 287_040 // $2,870.40
	PriceBusinessMonthlyCents = 49_900  // $499
	PriceBusinessAnnualCents  = 479_040 // $4,790.40

	// Per-plan breakeven thresholds for plan-recommendation logic (micro-USD).
	CreditFreeMicrousd     int64 = 1_000_000   // $1.00
	CreditStarterMicrousd  int64 = 19_000_000  // $19
	CreditProMicrousd      int64 = 99_000_000  // $99
	CreditScaleMicrousd    int64 = 299_000_000 // $299
	CreditBusinessMicrousd int64 = 499_000_000 // $499

	// Monthly run caps per plan (orchestration-only model).
	MaxRunsPerMonthFree       = 1_000
	MaxRunsPerMonthStarter    = 25_000
	MaxRunsPerMonthPro        = 250_000
	MaxRunsPerMonthScale      = 1_500_000
	MaxRunsPerMonthBusiness   = 10_000_000
	MaxRunsPerMonthEnterprise = -1 // unlimited

	// Concurrent run limits per plan.
	ConcurrentFree       = 5
	ConcurrentStarter    = 25
	ConcurrentPro        = 100
	ConcurrentScale      = 500
	ConcurrentBusiness   = 2_000
	ConcurrentEnterprise = -1 // unlimited

	// Overage cost per 1K runs in micro-USD.
	DefaultOveragePerKRunsMicrousd int64 = 500_000 // $0.50/1K
	FreeOveragePerKMicrousd        int64 = 500_000 // $0.50/1K
	StarterOveragePerKMicrousd     int64 = 500_000 // $0.50/1K
	ProOveragePerKMicrousd         int64 = 400_000 // $0.40/1K
	ScaleOveragePerKMicrousd       int64 = 200_000 // $0.20/1K
	BusinessOveragePerKMicrousd    int64 = 60_000  // $0.06/1K
	EnterpriseOveragePerKMicrousd  int64 = 30_000  // $0.03/1K

	// Data retention in days.
	RetentionFree       = 7
	RetentionStarter    = 30
	RetentionPro        = 90
	RetentionScale      = 180
	RetentionBusiness   = 365
	RetentionEnterprise = -1 // unlimited

	// Organization limits.
	MaxOrgsFree    = 1
	MaxOrgsStarter = 2
	MaxOrgsPro     = 5
	MaxOrgsScale   = 10

	MaxProjectsFree    = 1
	MaxProjectsStarter = 3
	MaxProjectsPro     = 10
	MaxProjectsScale   = 50

	MaxMembersFree    = 1
	MaxMembersStarter = 3
	MaxMembersPro     = 10
	MaxMembersScale   = 50

	// Spending limit caps per tier in micro-USD.
	MaxSpendingStarter int64 = 500_000_000   // $500
	MaxSpendingPro     int64 = 2_000_000_000 // $2,000
	MaxSpendingScale   int64 = 5_000_000_000 // $5,000

	// Total available regions (used when AllowedRegions is nil = all).
	TotalRegions = 25

	// Workflow DAG step limits per plan.
	MaxDAGStepsFree    = 10
	MaxDAGStepsStarter = 25
	MaxDAGStepsPro     = 100
	MaxDAGStepsScale   = 500

	// Scheduled job (cron) limits per plan.
	MaxScheduledFree    = 5
	MaxScheduledStarter = 25
	MaxScheduledPro     = 100
	MaxScheduledScale   = 500

	// Dispatch priority caps per plan (0 = default priority only).
	MaxDispatchPriorityFree       = 0
	MaxDispatchPriorityStarter    = 0
	MaxDispatchPriorityPro        = 10
	MaxDispatchPriorityScale      = 50
	MaxDispatchPriorityEnterprise = -1 // unlimited

	// Worker connection limits per plan (-1 = unlimited).
	WorkerConnectionsFree       = 1
	WorkerConnectionsStarter    = 5
	WorkerConnectionsPro        = 25
	WorkerConnectionsScale      = 100
	WorkerConnectionsEnterprise = -1 // unlimited

	// API rate limits (requests per minute).
	APIRateFree    = 60
	APIRateStarter = 300
	APIRatePro     = 1000
	APIRateScale   = 3000
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
		MaxRunsPerDay:           -1, // no daily cap; monthly cap applies
		MaxRunsPerMonth:         MaxRunsPerMonthFree,
		OveragePerKMicrousd:     FreeOveragePerKMicrousd,
		MaxConcurrentRuns:       ConcurrentFree,
		RetentionDays:           RetentionFree,
		AllowedRegions:          []string{"iad"},
		MaxAlertRulesPerProj:    0,
		MaxWebhookSubsPerProj:   0,
		MaxLogDrainsPerOrg:      0,
		MaxAIModelCallsPerDay:   20,
		AIAssistantBYOK:         false,
		HasRBAC:                 false,
		RBACLevel:               "",
		HasAuditLogs:            false,
		HasSSO:                  false,
		HasSLA:                  false,
		RequiresCreditCard:      false,
		OveragePerKRunsMicrousd: FreeOveragePerKMicrousd,
		AllowsHTTPMode:          true,
		LogStreamingEnabled:     false,
		MaxDispatchPriority:     MaxDispatchPriorityFree,
		WorkerConnections:       WorkerConnectionsFree,
		SupportLevel:            "community",
		MaxWorkflowDAGSteps:     MaxDAGStepsFree,
		HasApprovalGates:        false,
		HasSubWorkflows:         false,
		HasJobChaining:          false,
		MaxJobChainDepth:        0,
		HasCompensatingTxns:     false,
		HasCanaryDeployments:    false,
		MaxScheduledJobs:        MaxScheduledFree,
		AllCronOverlapPolicies:  false,
		MaxEnvironments:         1,
		MaxWebhookEndpoints:     0,
		WebhookEventLevel:       "none",
		APIRateLimit:            APIRateFree,
		MaxAddonPacks:           nil, // no addons on free tier
	},
	domain.PlanStarter: {
		PlanTier:                domain.PlanStarter,
		DisplayName:             "Starter",
		PriceMonthlyUsd:         PriceStarterMonthlyCents,
		PriceAnnualUsd:          PriceStarterAnnualCents,
		MaxOrgsPerUser:          MaxOrgsStarter,
		MaxProjectsPerOrg:       MaxProjectsStarter,
		MaxMembersPerOrg:        MaxMembersStarter,
		MaxRunsPerDay:           -1, // no daily cap; monthly cap applies
		MaxRunsPerMonth:         MaxRunsPerMonthStarter,
		OveragePerKMicrousd:     StarterOveragePerKMicrousd,
		MaxConcurrentRuns:       ConcurrentStarter,
		RetentionDays:           RetentionStarter,
		AllowedRegions:          nil,
		MaxAlertRulesPerProj:    0,
		MaxWebhookSubsPerProj:   3,
		MaxLogDrainsPerOrg:      1,
		MaxAIModelCallsPerDay:   100,
		AIAssistantBYOK:         false,
		HasRBAC:                 true,
		RBACLevel:               "basic",
		HasAuditLogs:            false,
		HasSSO:                  false,
		HasSLA:                  false,
		RequiresCreditCard:      true,
		OveragePerKRunsMicrousd: StarterOveragePerKMicrousd,
		AllowsHTTPMode:          true,
		LogStreamingEnabled:     true,
		MaxDispatchPriority:     MaxDispatchPriorityStarter,
		WorkerConnections:       WorkerConnectionsStarter,
		SupportLevel:            "email_72h",
		MaxWorkflowDAGSteps:     MaxDAGStepsStarter,
		HasApprovalGates:        false,
		HasSubWorkflows:         false,
		HasJobChaining:          false,
		MaxJobChainDepth:        0,
		HasCompensatingTxns:     false,
		HasCanaryDeployments:    false,
		MaxScheduledJobs:        MaxScheduledStarter,
		AllCronOverlapPolicies:  true,
		MaxEnvironments:         3,
		MaxWebhookEndpoints:     3,
		WebhookEventLevel:       "basic",
		APIRateLimit:            APIRateStarter,
		MaxAddonPacks: map[AddonType]int{
			AddonConcurrency100:    2,
			AddonLogDrain10GB:      2,
			AddonHistory30d:        2,
			AddonComplianceArchive: 0,
			AddonDedicatedWorkers:  0,
			AddonEnvironments5:     2,
			AddonConcurrentRuns:    2,
			AddonMembers:           2,
			AddonCronSchedules:     2,
			AddonDataRetention:     2,
			AddonWebhookEndpoints:  2,
		},
	},
	domain.PlanPro: {
		PlanTier:                domain.PlanPro,
		DisplayName:             "Pro",
		PriceMonthlyUsd:         PriceProMonthlyCents,
		PriceAnnualUsd:          PriceProAnnualCents,
		MaxOrgsPerUser:          MaxOrgsPro,
		MaxProjectsPerOrg:       MaxProjectsPro,
		MaxMembersPerOrg:        MaxMembersPro,
		MaxRunsPerDay:           -1, // no daily cap; monthly cap applies
		MaxRunsPerMonth:         MaxRunsPerMonthPro,
		OveragePerKMicrousd:     ProOveragePerKMicrousd,
		MaxConcurrentRuns:       ConcurrentPro,
		RetentionDays:           RetentionPro,
		AllowedRegions:          nil,
		MaxAlertRulesPerProj:    50,
		MaxWebhookSubsPerProj:   10,
		MaxLogDrainsPerOrg:      5,
		MaxAIModelCallsPerDay:   500,
		AIAssistantBYOK:         true,
		HasRBAC:                 true,
		RBACLevel:               "full",
		HasAuditLogs:            false,
		HasSSO:                  false,
		HasSLA:                  false,
		RequiresCreditCard:      true,
		OveragePerKRunsMicrousd: ProOveragePerKMicrousd,
		AllowsHTTPMode:          true,
		LogStreamingEnabled:     true,
		MaxDispatchPriority:     MaxDispatchPriorityPro,
		WorkerConnections:       WorkerConnectionsPro,
		SupportLevel:            "priority_24h",
		MaxWorkflowDAGSteps:     MaxDAGStepsPro,
		HasApprovalGates:        true,
		HasSubWorkflows:         true,
		HasJobChaining:          true,
		MaxJobChainDepth:        10,
		HasCompensatingTxns:     true,
		HasCanaryDeployments:    false,
		MaxScheduledJobs:        MaxScheduledPro,
		AllCronOverlapPolicies:  true,
		MaxEnvironments:         3,
		MaxWebhookEndpoints:     10,
		WebhookEventLevel:       "all",
		APIRateLimit:            APIRatePro,
		MaxAddonPacks: map[AddonType]int{
			AddonConcurrency100:    5,
			AddonLogDrain10GB:      5,
			AddonHistory30d:        5,
			AddonComplianceArchive: 0,
			AddonDedicatedWorkers:  5,
			AddonEnvironments5:     5,
			AddonConcurrentRuns:    5,
			AddonMembers:           5,
			AddonCronSchedules:     5,
			AddonDataRetention:     5,
			AddonWebhookEndpoints:  5,
		},
	},
	domain.PlanScale: {
		PlanTier:                domain.PlanScale,
		DisplayName:             "Scale",
		PriceMonthlyUsd:         PriceScaleMonthlyCents,
		PriceAnnualUsd:          PriceScaleAnnualCents,
		MaxOrgsPerUser:          MaxOrgsScale,
		MaxProjectsPerOrg:       MaxProjectsScale,
		MaxMembersPerOrg:        MaxMembersScale,
		MaxRunsPerDay:           -1, // no daily cap; monthly cap applies
		MaxRunsPerMonth:         MaxRunsPerMonthScale,
		OveragePerKMicrousd:     ScaleOveragePerKMicrousd,
		MaxConcurrentRuns:       ConcurrentScale,
		RetentionDays:           RetentionScale,
		AllowedRegions:          nil,
		MaxAlertRulesPerProj:    50,
		MaxWebhookSubsPerProj:   25,
		MaxLogDrainsPerOrg:      10,
		MaxAIModelCallsPerDay:   1000,
		AIAssistantBYOK:         true,
		HasRBAC:                 true,
		RBACLevel:               "full",
		HasAuditLogs:            true,
		HasSSO:                  false,
		HasSLA:                  false,
		RequiresCreditCard:      true,
		OveragePerKRunsMicrousd: ScaleOveragePerKMicrousd,
		AllowsHTTPMode:          true,
		LogStreamingEnabled:     true,
		MaxDispatchPriority:     MaxDispatchPriorityScale,
		WorkerConnections:       WorkerConnectionsScale,
		SupportLevel:            "priority_slack_8h",
		MaxWorkflowDAGSteps:     MaxDAGStepsScale,
		HasApprovalGates:        true,
		HasSubWorkflows:         true,
		HasJobChaining:          true,
		MaxJobChainDepth:        10,
		HasCompensatingTxns:     true,
		HasCanaryDeployments:    true,
		MaxScheduledJobs:        MaxScheduledScale,
		AllCronOverlapPolicies:  true,
		MaxEnvironments:         3,
		MaxWebhookEndpoints:     25,
		WebhookEventLevel:       "all",
		APIRateLimit:            APIRateScale,
		MaxAddonPacks: map[AddonType]int{
			AddonConcurrency100:    10,
			AddonLogDrain10GB:      10,
			AddonHistory30d:        10,
			AddonComplianceArchive: 1,
			AddonDedicatedWorkers:  10,
			AddonEnvironments5:     10,
			AddonConcurrentRuns:    10,
			AddonMembers:           10,
			AddonCronSchedules:     10,
			AddonDataRetention:     10,
			AddonWebhookEndpoints:  10,
		},
	},
	domain.PlanBusiness: {
		PlanTier:                domain.PlanBusiness,
		DisplayName:             "Business",
		PriceMonthlyUsd:         PriceBusinessMonthlyCents,
		PriceAnnualUsd:          PriceBusinessAnnualCents,
		MaxOrgsPerUser:          -1,
		MaxProjectsPerOrg:       -1,
		MaxMembersPerOrg:        -1,
		MaxRunsPerDay:           -1,
		MaxRunsPerMonth:         MaxRunsPerMonthBusiness,
		OveragePerKMicrousd:     BusinessOveragePerKMicrousd,
		MaxConcurrentRuns:       ConcurrentBusiness,
		RetentionDays:           RetentionBusiness,
		AllowedRegions:          nil,
		MaxAlertRulesPerProj:    -1,
		MaxWebhookSubsPerProj:   -1,
		MaxLogDrainsPerOrg:      -1,
		MaxAIModelCallsPerDay:   -1,
		AIAssistantBYOK:         true,
		HasRBAC:                 true,
		RBACLevel:               "advanced",
		HasAuditLogs:            true,
		HasSSO:                  true,
		HasSLA:                  true,
		RequiresCreditCard:      true,
		OveragePerKRunsMicrousd: BusinessOveragePerKMicrousd,
		AllowsHTTPMode:          true,
		LogStreamingEnabled:     true,
		MaxDispatchPriority:     -1,
		WorkerConnections:       -1,
		SupportLevel:            "priority_slack_8h",
		MaxWorkflowDAGSteps:     -1,
		HasApprovalGates:        true,
		HasSubWorkflows:         true,
		HasJobChaining:          true,
		MaxJobChainDepth:        -1,
		HasCompensatingTxns:     true,
		HasCanaryDeployments:    true,
		HasSCIM:                 true,
		HasIPAllowlisting:       true,
		HasSessionManagement:    true,
		HasSecretRotation:       true,
		HasSIEMExport:           true,
		HasPriorityQueue:        true,
		MaxScheduledJobs:        -1,
		AllCronOverlapPolicies:  true,
		MaxEnvironments:         25,
		MaxWebhookEndpoints:     -1,
		WebhookEventLevel:       "all",
		APIRateLimit:            -1,
		MaxAddonPacks: map[AddonType]int{
			AddonConcurrency100:    -1,
			AddonLogDrain10GB:      -1,
			AddonHistory30d:        -1,
			AddonComplianceArchive: 1,
			AddonDedicatedWorkers:  -1,
			AddonEnvironments5:     -1,
			AddonConcurrentRuns:    -1,
			AddonMembers:           -1,
			AddonCronSchedules:     -1,
			AddonDataRetention:     -1,
			AddonWebhookEndpoints:  -1,
		},
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
		MaxRunsPerMonth:         MaxRunsPerMonthEnterprise,
		OveragePerKMicrousd:     EnterpriseOveragePerKMicrousd,
		MaxConcurrentRuns:       ConcurrentEnterprise,
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
		OveragePerKRunsMicrousd: EnterpriseOveragePerKMicrousd,
		AllowsHTTPMode:          true,
		LogStreamingEnabled:     true,
		MaxDispatchPriority:     MaxDispatchPriorityEnterprise,
		WorkerConnections:       WorkerConnectionsEnterprise,
		SupportLevel:            "dedicated",
		MaxWorkflowDAGSteps:     -1,
		HasApprovalGates:        true,
		HasSubWorkflows:         true,
		HasJobChaining:          true,
		MaxJobChainDepth:        -1,
		HasCompensatingTxns:     true,
		HasCanaryDeployments:    true,
		HasDedicatedCompute:     true,
		HasStaticIPs:            true,
		HasVPCPeering:           true,
		HasSCIM:                 true,
		HasDataResidency:        true,
		HasCustomRBAC:           true,
		HasPriorityQueue:        true,
		HasIPAllowlisting:       true,
		HasSessionManagement:    true,
		HasSecretRotation:       true,
		HasSIEMExport:           true,
		MaxScheduledJobs:        -1,
		AllCronOverlapPolicies:  true,
		MaxEnvironments:         -1,
		MaxWebhookEndpoints:     -1,
		WebhookEventLevel:       "all_custom",
		APIRateLimit:            -1,
		MaxAddonPacks: map[AddonType]int{
			AddonConcurrency100:    -1,
			AddonLogDrain10GB:      -1,
			AddonHistory30d:        -1,
			AddonComplianceArchive: -1,
			AddonDedicatedWorkers:  -1,
			AddonEnvironments5:     -1,
			AddonConcurrentRuns:    -1,
			AddonMembers:           -1,
			AddonCronSchedules:     -1,
			AddonDataRetention:     -1,
			AddonWebhookEndpoints:  -1,
		},
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
		lessInt(oldLimits.MaxRunsPerMonth, newLimits.MaxRunsPerMonth) ||
		lessInt(oldLimits.MaxConcurrentRuns, newLimits.MaxConcurrentRuns) ||
		lessInt(oldLimits.MaxProjectsPerOrg, newLimits.MaxProjectsPerOrg) ||
		lessInt(oldLimits.MaxMembersPerOrg, newLimits.MaxMembersPerOrg) ||
		lessInt(oldLimits.MaxOrgsPerUser, newLimits.MaxOrgsPerUser) ||
		lessInt(oldLimits.RetentionDays, newLimits.RetentionDays) ||
		lessInt(oldLimits.MaxAlertRulesPerProj, newLimits.MaxAlertRulesPerProj) ||
		lessInt(oldLimits.MaxWebhookSubsPerProj, newLimits.MaxWebhookSubsPerProj) ||
		lessInt(oldLimits.MaxLogDrainsPerOrg, newLimits.MaxLogDrainsPerOrg) ||
		lessInt(oldLimits.MaxAIModelCallsPerDay, newLimits.MaxAIModelCallsPerDay) ||
		lessInt(oldLimits.MaxWorkflowDAGSteps, newLimits.MaxWorkflowDAGSteps) ||
		lessInt(oldLimits.MaxScheduledJobs, newLimits.MaxScheduledJobs) ||
		lessInt(oldLimits.MaxEnvironments, newLimits.MaxEnvironments) ||
		lessInt(oldLimits.MaxWebhookEndpoints, newLimits.MaxWebhookEndpoints) ||
		lessInt(oldLimits.APIRateLimit, newLimits.APIRateLimit) ||
		lessInt(oldLimits.MaxDispatchPriority, newLimits.MaxDispatchPriority) ||
		lessInt(oldLimits.WorkerConnections, newLimits.WorkerConnections)
}

// GetPlanLimits returns the plan limits for the given tier.
// Returns free plan limits if the tier is unknown.
func GetPlanLimits(tier domain.PlanTier) OrgPlanLimits {
	if limits, ok := Plans[tier]; ok {
		return limits
	}
	return Plans[domain.PlanFree]
}
