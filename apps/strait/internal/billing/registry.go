package billing

import "strait/internal/domain"

// Feature represents a plan-gated feature that can be checked via the registry.
type Feature string

const (
	FeatureHTTPMode          Feature = "http_mode"
	FeatureApprovalGates     Feature = "approval_gates"
	FeatureSubWorkflows      Feature = "sub_workflows"
	FeatureJobChaining       Feature = "job_chaining"
	FeatureCompensatingTxns  Feature = "compensating_txns"
	FeatureCanaryDeployments Feature = "canary_deployments"
	FeatureAuditLogs         Feature = "audit_logs"
	FeatureSSO               Feature = "sso"
	FeatureSLA               Feature = "sla"
	FeatureRBAC              Feature = "rbac"
	FeatureAllCronOverlap    Feature = "all_cron_overlap_policies"
	FeatureDedicatedCompute  Feature = "dedicated_compute"
	FeatureStaticIPs         Feature = "static_ips"
	FeatureVPCPeering        Feature = "vpc_peering"
	FeatureSCIM              Feature = "scim"
	FeatureDataResidency     Feature = "data_residency"
	FeatureCustomRBAC        Feature = "custom_rbac"
	FeaturePriorityQueue     Feature = "priority_queue"
	FeatureIPAllowlisting    Feature = "ip_allowlisting"
	FeatureSessionManagement Feature = "session_management"
	FeatureSecretRotation    Feature = "secret_rotation"
	FeatureSIEMExport        Feature = "siem_export"
	FeatureLogStreaming      Feature = "log_streaming"
)

// LimitKey represents a numeric plan limit that can be queried via the registry.
type LimitKey string

const (
	LimitMaxProjectsPerOrg   LimitKey = "max_projects_per_org"
	LimitMaxMembersPerOrg    LimitKey = "max_members_per_org"
	LimitMaxConcurrentRuns   LimitKey = "max_concurrent_runs"
	LimitMaxRunsPerDay       LimitKey = "max_runs_per_day"
	LimitMaxOrgsPerUser      LimitKey = "max_orgs_per_user"
	LimitRetentionDays       LimitKey = "retention_days"
	LimitMaxWorkflowDAGSteps LimitKey = "max_workflow_dag_steps"
	LimitMaxJobChainDepth    LimitKey = "max_job_chain_depth"
	LimitMaxScheduledJobs    LimitKey = "max_scheduled_jobs"
	LimitMaxEnvironments     LimitKey = "max_environments"
	LimitMaxWebhookEndpoints LimitKey = "max_webhook_endpoints"
	LimitMaxAlertRules       LimitKey = "max_alert_rules_per_proj"
	LimitMaxWebhookSubs      LimitKey = "max_webhook_subs_per_proj"
	LimitMaxLogDrains        LimitKey = "max_log_drains_per_org"
	LimitMaxNotificationCh   LimitKey = "max_notification_channels"
	LimitAPIRateLimit        LimitKey = "api_rate_limit"
	LimitWorkerConnections   LimitKey = "worker_connections"
	LimitMaxDispatchPriority LimitKey = "max_dispatch_priority"
)

// PlanRegistry provides plan definitions and feature checks.
// Backed by the static Plans map today; can be swapped for DB-backed
// or remote-config-backed implementations later.
type PlanRegistry interface {
	// Get returns the full plan limits for a tier. Unknown tiers return Free.
	Get(tier domain.PlanTier) OrgPlanLimits

	// All returns all plan definitions in tier order (free -> enterprise).
	All() []OrgPlanLimits

	// AllowsFeature returns true if the given tier has the specified feature.
	AllowsFeature(tier domain.PlanTier, feature Feature) bool

	// MaxForLimit returns the numeric limit value for a tier and limit key.
	// Returns 0 for unknown limit keys, -1 for unlimited.
	MaxForLimit(tier domain.PlanTier, limit LimitKey) int

	// RequiredPlanForFeature returns the minimum plan tier that includes a
	// launch-active feature. Roadmap-only features return the empty tier because
	// no launch plan includes them.
	RequiredPlanForFeature(feature Feature) domain.PlanTier
}

// StaticRegistry implements PlanRegistry backed by the in-memory Plans map.
type StaticRegistry struct{}

// NewStaticRegistry returns a new static plan registry.
func NewStaticRegistry() *StaticRegistry {
	return &StaticRegistry{}
}

func (r *StaticRegistry) Get(tier domain.PlanTier) OrgPlanLimits {
	return GetPlanLimits(tier)
}

func (r *StaticRegistry) All() []OrgPlanLimits {
	result := make([]OrgPlanLimits, 0, len(Plans))
	for _, tier := range domain.AllPlanTiers() {
		if limits, ok := Plans[tier]; ok {
			result = append(result, limits)
		}
	}
	return result
}

func (r *StaticRegistry) AllowsFeature(tier domain.PlanTier, feature Feature) bool {
	limits := GetPlanLimits(tier)
	switch feature {
	case FeatureHTTPMode:
		return limits.AllowsHTTPMode
	case FeatureApprovalGates:
		return limits.HasApprovalGates
	case FeatureSubWorkflows:
		return limits.HasSubWorkflows
	case FeatureJobChaining:
		return limits.HasJobChaining
	case FeatureCompensatingTxns:
		return limits.HasCompensatingTxns
	case FeatureCanaryDeployments:
		return limits.HasCanaryDeployments
	case FeatureAuditLogs:
		return limits.HasAuditLogs
	case FeatureSSO:
		return limits.HasSSO
	case FeatureSLA:
		return limits.HasSLA
	case FeatureRBAC:
		return limits.HasRBAC
	case FeatureAllCronOverlap:
		return limits.AllCronOverlapPolicies
	case FeatureDedicatedCompute:
		return limits.HasDedicatedCompute
	case FeatureStaticIPs:
		return limits.HasStaticIPs
	case FeatureVPCPeering:
		return limits.HasVPCPeering
	case FeatureSCIM:
		return limits.HasSCIM
	case FeatureDataResidency:
		return limits.HasDataResidency
	case FeatureCustomRBAC:
		return limits.HasCustomRBAC
	case FeaturePriorityQueue:
		return limits.HasPriorityQueue
	case FeatureIPAllowlisting:
		return limits.HasIPAllowlisting
	case FeatureSessionManagement:
		return limits.HasSessionManagement
	case FeatureSecretRotation:
		return limits.HasSecretRotation
	case FeatureSIEMExport:
		return limits.HasSIEMExport
	case FeatureLogStreaming:
		return limits.LogStreamingEnabled
	default:
		return false
	}
}

// IsRoadmapFeature returns true for features known to the catalog but inactive
// for launch. Roadmap features must not produce self-serve upgrade CTAs.
func IsRoadmapFeature(feature Feature) bool {
	switch feature {
	case FeatureSSO,
		FeatureDedicatedCompute,
		FeatureStaticIPs,
		FeatureVPCPeering,
		FeatureSCIM,
		FeatureDataResidency,
		FeatureCustomRBAC,
		FeaturePriorityQueue,
		FeatureIPAllowlisting,
		FeatureSessionManagement,
		FeatureSecretRotation,
		FeatureSIEMExport:
		return true
	default:
		return false
	}
}

// RequiredPlanForFeature returns the minimum plan tier that includes the given feature.
// Returns the empty tier for launch-roadmap features and PlanEnterprise for
// unknown features as a safe default.
func (r *StaticRegistry) RequiredPlanForFeature(feature Feature) domain.PlanTier {
	if IsRoadmapFeature(feature) {
		return ""
	}
	for _, tier := range domain.AllPlanTiers() {
		if r.AllowsFeature(tier, feature) {
			return tier
		}
	}
	return domain.PlanEnterprise
}

func (r *StaticRegistry) MaxForLimit(tier domain.PlanTier, limit LimitKey) int {
	limits := GetPlanLimits(tier)
	switch limit {
	case LimitMaxProjectsPerOrg:
		return limits.MaxProjectsPerOrg
	case LimitMaxMembersPerOrg:
		return limits.MaxMembersPerOrg
	case LimitMaxConcurrentRuns:
		return limits.MaxConcurrentRuns
	case LimitMaxRunsPerDay:
		return int(limits.MaxRunsPerDay)
	case LimitMaxOrgsPerUser:
		return limits.MaxOrgsPerUser
	case LimitRetentionDays:
		return limits.RetentionDays
	case LimitMaxWorkflowDAGSteps:
		return limits.MaxWorkflowDAGSteps
	case LimitMaxJobChainDepth:
		return limits.MaxJobChainDepth
	case LimitMaxScheduledJobs:
		return limits.MaxScheduledJobs
	case LimitMaxEnvironments:
		return limits.MaxEnvironments
	case LimitMaxWebhookEndpoints:
		return limits.MaxWebhookEndpoints
	case LimitMaxAlertRules:
		return limits.MaxAlertRulesPerProj
	case LimitMaxWebhookSubs:
		return limits.MaxWebhookSubsPerProj
	case LimitMaxLogDrains:
		return limits.MaxLogDrainsPerOrg
	case LimitMaxNotificationCh:
		return limits.MaxNotificationChannels
	case LimitAPIRateLimit:
		return limits.APIRateLimit
	case LimitWorkerConnections:
		return limits.WorkerConnections
	case LimitMaxDispatchPriority:
		return limits.MaxDispatchPriority
	default:
		return 0
	}
}
