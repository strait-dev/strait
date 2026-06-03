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
	MaxRunsPerMonth          int      `json:"max_runs_per_month"`
	MaxConcurrentRuns        int      `json:"max_concurrent_runs"`
	ComputeCreditMicrousd    int64    `json:"compute_credit_microusd"`
	RetentionDays            int      `json:"retention_days"`
	AllowedRegions           []string `json:"allowed_regions"`
	MaxWebhookSubsPerProject int      `json:"max_webhook_subs_per_project"`
	MaxLogDrainsPerOrg       int      `json:"max_log_drains_per_org"`
	HasRBAC                  bool     `json:"has_rbac"`
	RBACLevel                string   `json:"rbac_level,omitempty"`
	HasAuditLogs             bool     `json:"has_audit_logs"`
	HasSLA                   bool     `json:"has_sla"`
	HasCanaryDeployments     bool     `json:"has_canary_deployments"`
	HasApprovalGates         bool     `json:"has_approval_gates"`
	HasSubWorkflows          bool     `json:"has_sub_workflows"`
	HasJobChaining           bool     `json:"has_job_chaining"`
	HasCompensatingTxns      bool     `json:"has_compensating_txns"`
	RequiresCreditCard       bool     `json:"requires_credit_card"`
	OveragePerKRunsMicrousd  int64    `json:"overage_per_k_runs_microusd"`
	SupportLevel             string   `json:"support_level"`
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

func planResponseForTier(tier domain.PlanTier) PlanResponse {
	limits := billing.GetPlanLimits(tier)
	catalog := billing.GetPlanCatalog(tier)
	return PlanResponse{
		Tier:                     string(limits.PlanTier),
		DisplayName:              limits.DisplayName,
		PriceMonthlyUSD:          limits.PriceMonthlyUsd,
		PriceAnnualUSD:           limits.PriceAnnualUsd,
		MaxOrgsPerUser:           limits.MaxOrgsPerUser,
		MaxProjectsPerOrg:        limits.MaxProjectsPerOrg,
		MaxMembersPerOrg:         limits.MaxMembersPerOrg,
		MaxRunsPerMonth:          limits.MaxRunsPerMonth,
		MaxConcurrentRuns:        limits.MaxConcurrentRuns,
		ComputeCreditMicrousd:    computeCreditForPlan(tier),
		RetentionDays:            limits.RetentionDays,
		AllowedRegions:           limits.AllowedRegions,
		MaxWebhookSubsPerProject: limits.MaxWebhookSubsPerProj,
		MaxLogDrainsPerOrg:       limits.MaxLogDrainsPerOrg,
		HasRBAC:                  limits.HasRBAC,
		RBACLevel:                limits.RBACLevel,
		HasAuditLogs:             limits.HasAuditLogs,
		HasSLA:                   limits.HasSLA,
		HasCanaryDeployments:     limits.HasCanaryDeployments,
		HasApprovalGates:         limits.HasApprovalGates,
		HasSubWorkflows:          limits.HasSubWorkflows,
		HasJobChaining:           limits.HasJobChaining,
		HasCompensatingTxns:      limits.HasCompensatingTxns,
		RequiresCreditCard:       limits.RequiresCreditCard,
		OveragePerKRunsMicrousd:  limits.OveragePerKMicrousd,
		SupportLevel:             limits.SupportLevel,
		MaxEnvironments:          limits.MaxEnvironments,
		MaxScheduledJobs:         limits.MaxScheduledJobs,
		CronMinIntervalSec:       limits.CronMinIntervalSec,
		MaxWebhookEndpoints:      limits.MaxWebhookEndpoints,
		MaxWorkflowDAGSteps:      limits.MaxWorkflowDAGSteps,
		APIRateLimit:             limits.APIRateLimit,
		WorkerConnections:        limits.WorkerConnections,
		RoadmapFeatures:          append([]string(nil), catalog.RoadmapFeatures...),
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
