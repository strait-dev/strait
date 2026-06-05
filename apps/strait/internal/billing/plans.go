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
	OveragePerKMicrousd     int64 // per-1K-run overage in micro-USD
	MaxConcurrentRuns       int   // -1 = unlimited
	RetentionDays           int
	AllowedRegions          []string // launch-active regions; nil falls back to the default region
	MaxAlertRulesPerProj    int      // -1 = unlimited, 0 = none
	MaxWebhookSubsPerProj   int      // -1 = unlimited, 0 = none
	MaxLogDrainsPerOrg      int      // -1 = unlimited
	MaxNotificationChannels int      // max notification channels per project; -1 = unlimited, 0 = none
	HasRBAC                 bool
	RBACLevel               string // "", "basic", "full", "advanced"
	HasAuditLogs            bool
	HasSSO                  bool
	HasSLA                  bool
	RequiresCreditCard      bool
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

	// Enterprise and roadmap feature gates. These stay false unless an
	// enforcement path exists for the launch catalog.
	HasDedicatedCompute  bool
	HasStaticIPs         bool
	HasVPCPeering        bool
	HasSCIM              bool
	HasDataResidency     bool
	HasCustomRBAC        bool
	HasPriorityQueue     bool
	HasIPAllowlisting    bool
	HasSessionManagement bool
	HasSecretRotation    bool
	HasSIEMExport        bool

	// Resource limits.
	MaxScheduledJobs       int               // max cron schedules; -1 = unlimited
	CronMinIntervalSec     int               // minimum interval between cron-triggered runs; 0 = no minimum
	AllCronOverlapPolicies bool              // false = "allow" only; true = all policies
	MaxEnvironments        int               // max environments per project; -1 = unlimited
	MaxWebhookEndpoints    int               // max webhook endpoints; -1 = unlimited, 0 = none
	WebhookEventLevel      string            // "none", "basic", "all", "all_custom"
	APIRateLimit           int               // requests per minute; -1 = unlimited
	MaxAddonPacks          map[AddonType]int `json:"max_addon_packs,omitempty"` // max packs per addon type; -1 = unlimited
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
		lessInt(oldLimits.MaxNotificationChannels, newLimits.MaxNotificationChannels) ||
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
