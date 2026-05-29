package api

import (
	"context"

	"strait/internal/billing"
	"strait/internal/domain"
)

type GetPlansInput struct{}

type GetPlansOutput struct {
	Body GetPlansOutputBody
}

type GetPlansOutputBody struct {
	Plans []PlanResponse `json:"plans"`
}

type PlanResponse struct {
	Tier                     string   `json:"tier"`
	DisplayName              string   `json:"display_name"`
	PriceMonthlyUSD          int      `json:"price_monthly_usd"`
	PriceAnnualUSD           int      `json:"price_annual_usd"`
	MaxOrgsPerUser           int      `json:"max_orgs_per_user"`
	MaxProjectsPerOrg        int      `json:"max_projects_per_org"`
	MaxMembersPerOrg         int      `json:"max_members_per_org"`
	MaxRunsPerDay            int64    `json:"max_runs_per_day"`
	MaxRunsPerMonth          int      `json:"max_runs_per_month"`
	MaxConcurrentRuns        int      `json:"max_concurrent_runs"`
	ComputeCreditMicrousd    int64    `json:"compute_credit_microusd"`
	RetentionDays            int      `json:"retention_days"`
	AllowedRegions           []string `json:"allowed_regions"`
	MaxAlertRulesPerProject  int      `json:"max_alert_rules_per_project"`
	MaxWebhookSubsPerProject int      `json:"max_webhook_subs_per_project"`
	MaxLogDrainsPerOrg       int      `json:"max_log_drains_per_org"`
	MaxAIModelCallsPerDay    int      `json:"max_ai_model_calls_per_day"`
	AIAssistantBYOK          bool     `json:"ai_assistant_byok"`
	HasRBAC                  bool     `json:"has_rbac"`
	RBACLevel                string   `json:"rbac_level,omitempty"`
	HasAuditLogs             bool     `json:"has_audit_logs"`
	HasSSO                   bool     `json:"has_sso"`
	HasSLA                   bool     `json:"has_sla"`
	RequiresCreditCard       bool     `json:"requires_credit_card"`
	OveragePerKRunsMicrousd  int64    `json:"overage_per_k_runs_microusd"`
	SupportLevel             string   `json:"support_level"`
	HasDedicatedCompute      bool     `json:"has_dedicated_compute"`
	HasStaticIPs             bool     `json:"has_static_ips"`
	HasVPCPeering            bool     `json:"has_vpc_peering"`
	HasSCIM                  bool     `json:"has_scim"`
	HasDataResidency         bool     `json:"has_data_residency"`
	HasCustomRBAC            bool     `json:"has_custom_rbac"`
	HasReservedCapacity      bool     `json:"has_reserved_capacity"`
	HasPriorityQueue         bool     `json:"has_priority_queue"`
	HasIPAllowlisting        bool     `json:"has_ip_allowlisting"`
	HasSessionManagement     bool     `json:"has_session_management"`
	HasSecretRotation        bool     `json:"has_secret_rotation"`
	HasSIEMExport            bool     `json:"has_siem_export"`
	MaxEnvironments          int      `json:"max_environments"`
	MaxScheduledJobs         int      `json:"max_scheduled_jobs"`
	CronMinIntervalSec       int      `json:"cron_min_interval_sec"`
	MaxWebhookEndpoints      int      `json:"max_webhook_endpoints"`
	MaxWorkflowDAGSteps      int      `json:"max_workflow_dag_steps"`
	APIRateLimit             int      `json:"api_rate_limit"`
	WorkerConnections        int      `json:"worker_connections"`
	RoadmapFeatures          []string `json:"roadmap_features"`
}

func (s *Server) handleGetPlans(_ context.Context, _ *GetPlansInput) (*GetPlansOutput, error) {
	plans := make([]PlanResponse, 0, len(orderedPlanTiers))
	for _, tier := range orderedPlanTiers {
		plans = append(plans, planResponseForTier(tier))
	}
	return &GetPlansOutput{Body: GetPlansOutputBody{Plans: plans}}, nil
}

var orderedPlanTiers = []domain.PlanTier{
	domain.PlanFree,
	domain.PlanStarter,
	domain.PlanPro,
	domain.PlanScale,
	domain.PlanBusiness,
	domain.PlanEnterprise,
}

var roadmapFeaturesByTier = map[domain.PlanTier][]string{
	domain.PlanBusiness: {
		"SSO/SAML",
		"SCIM",
		"IP allowlisting",
		"static IPs",
		"VPC peering",
		"data residency",
	},
	domain.PlanEnterprise: {
		"SSO/SAML",
		"SCIM",
		"IP allowlisting",
		"static IPs",
		"VPC peering",
		"data residency",
		"single-tenant orchestration",
		"BYO-cloud",
	},
}

func planResponseForTier(tier domain.PlanTier) PlanResponse {
	limits := billing.GetPlanLimits(tier)
	return PlanResponse{
		Tier:                     string(limits.PlanTier),
		DisplayName:              limits.DisplayName,
		PriceMonthlyUSD:          limits.PriceMonthlyUsd,
		PriceAnnualUSD:           limits.PriceAnnualUsd,
		MaxOrgsPerUser:           limits.MaxOrgsPerUser,
		MaxProjectsPerOrg:        limits.MaxProjectsPerOrg,
		MaxMembersPerOrg:         limits.MaxMembersPerOrg,
		MaxRunsPerDay:            limits.MaxRunsPerDay,
		MaxRunsPerMonth:          limits.MaxRunsPerMonth,
		MaxConcurrentRuns:        limits.MaxConcurrentRuns,
		ComputeCreditMicrousd:    computeCreditForPlan(tier),
		RetentionDays:            limits.RetentionDays,
		AllowedRegions:           limits.AllowedRegions,
		MaxAlertRulesPerProject:  limits.MaxAlertRulesPerProj,
		MaxWebhookSubsPerProject: limits.MaxWebhookSubsPerProj,
		MaxLogDrainsPerOrg:       limits.MaxLogDrainsPerOrg,
		MaxAIModelCallsPerDay:    limits.MaxAIModelCallsPerDay,
		AIAssistantBYOK:          limits.AIAssistantBYOK,
		HasRBAC:                  limits.HasRBAC,
		RBACLevel:                limits.RBACLevel,
		HasAuditLogs:             limits.HasAuditLogs,
		HasSSO:                   limits.HasSSO,
		HasSLA:                   limits.HasSLA,
		RequiresCreditCard:       limits.RequiresCreditCard,
		OveragePerKRunsMicrousd:  limits.OveragePerKMicrousd,
		SupportLevel:             limits.SupportLevel,
		HasDedicatedCompute:      limits.HasDedicatedCompute,
		HasStaticIPs:             limits.HasStaticIPs,
		HasVPCPeering:            limits.HasVPCPeering,
		HasSCIM:                  limits.HasSCIM,
		HasDataResidency:         limits.HasDataResidency,
		HasCustomRBAC:            limits.HasCustomRBAC,
		HasReservedCapacity:      false,
		HasPriorityQueue:         limits.HasPriorityQueue,
		HasIPAllowlisting:        limits.HasIPAllowlisting,
		HasSessionManagement:     limits.HasSessionManagement,
		HasSecretRotation:        limits.HasSecretRotation,
		HasSIEMExport:            limits.HasSIEMExport,
		MaxEnvironments:          limits.MaxEnvironments,
		MaxScheduledJobs:         limits.MaxScheduledJobs,
		CronMinIntervalSec:       limits.CronMinIntervalSec,
		MaxWebhookEndpoints:      limits.MaxWebhookEndpoints,
		MaxWorkflowDAGSteps:      limits.MaxWorkflowDAGSteps,
		APIRateLimit:             limits.APIRateLimit,
		WorkerConnections:        limits.WorkerConnections,
		RoadmapFeatures:          roadmapFeaturesByTier[tier],
	}
}

func computeCreditForPlan(tier domain.PlanTier) int64 {
	switch tier {
	case domain.PlanFree:
		return billing.CreditFreeMicrousd
	case domain.PlanStarter:
		return billing.CreditStarterMicrousd
	case domain.PlanPro:
		return billing.CreditProMicrousd
	case domain.PlanScale:
		return billing.CreditScaleMicrousd
	case domain.PlanBusiness:
		return billing.CreditBusinessMicrousd
	default:
		return 0
	}
}
