package api

import (
	"context"

	"strait/internal/billing"
	"strait/internal/domain"
)

type planResponse struct {
	Tier                    string   `json:"tier"`
	DisplayName             string   `json:"display_name"`
	PriceMonthlyUsd         int      `json:"price_monthly_usd"`
	PriceAnnualUsd          int      `json:"price_annual_usd"`
	MaxOrgsPerUser          int      `json:"max_orgs_per_user"`
	MaxProjectsPerOrg       int      `json:"max_projects_per_org"`
	MaxMembersPerOrg        int      `json:"max_members_per_org"`
	MaxRunsPerDay         int64  `json:"max_runs_per_day"`
	MaxConcurrentRuns     int    `json:"max_concurrent_runs"`
	FreeManagedPreset     string `json:"free_managed_preset,omitempty"`
	FreeManagedMaxTimeout int    `json:"free_managed_max_timeout,omitempty"`
	RetentionDays           int      `json:"retention_days"`
	AllowedRegions          []string `json:"allowed_regions"`
	MaxAlertRulesPerProj    int      `json:"max_alert_rules_per_project"`
	MaxWebhookSubsPerProj   int      `json:"max_webhook_subs_per_project"`
	MaxLogDrainsPerOrg      int      `json:"max_log_drains_per_org"`
	MaxAIModelCallsPerDay   int      `json:"max_ai_model_calls_per_day"`
	AIAssistantBYOK         bool     `json:"ai_assistant_byok"`
	HasRBAC                 bool     `json:"has_rbac"`
	RBACLevel               string   `json:"rbac_level,omitempty"`
	HasAuditLogs            bool     `json:"has_audit_logs"`
	HasSSO                  bool     `json:"has_sso"`
	HasSLA                  bool     `json:"has_sla"`
	RequiresCreditCard      bool     `json:"requires_credit_card"`
	OveragePerKRunsMicrousd int64    `json:"overage_per_k_runs_microusd"`
	SupportLevel            string   `json:"support_level"`
	HasDedicatedCompute     bool     `json:"has_dedicated_compute"`
	HasStaticIPs            bool     `json:"has_static_ips"`
	HasVPCPeering           bool     `json:"has_vpc_peering"`
	HasSCIM                 bool     `json:"has_scim"`
	HasDataResidency        bool     `json:"has_data_residency"`
	HasCustomRBAC           bool     `json:"has_custom_rbac"`
	HasReservedCapacity     bool     `json:"has_reserved_capacity"`
	HasPriorityQueue        bool     `json:"has_priority_queue"`
	HasIPAllowlisting       bool     `json:"has_ip_allowlisting"`
	HasSessionManagement    bool     `json:"has_session_management"`
	HasSecretRotation       bool     `json:"has_secret_rotation"`
	HasSIEMExport           bool     `json:"has_siem_export"`
}

func toPlanResponse(p billing.OrgPlanLimits) planResponse {
	regions := p.AllowedRegions
	if regions == nil {
		regions = []string{} // "all" represented as empty array with -1 convention
	}

	return planResponse{
		Tier:                    string(p.PlanTier),
		DisplayName:             p.DisplayName,
		PriceMonthlyUsd:         p.PriceMonthlyUsd,
		PriceAnnualUsd:          p.PriceAnnualUsd,
		MaxOrgsPerUser:          p.MaxOrgsPerUser,
		MaxProjectsPerOrg:       p.MaxProjectsPerOrg,
		MaxMembersPerOrg:        p.MaxMembersPerOrg,
		MaxRunsPerDay:         p.MaxRunsPerDay,
		MaxConcurrentRuns:     p.MaxConcurrentRuns,
		FreeManagedPreset:     p.FreeManagedPreset,
		FreeManagedMaxTimeout: p.FreeManagedMaxTimeout,
		RetentionDays:           p.RetentionDays,
		AllowedRegions:          regions,
		MaxAlertRulesPerProj:    p.MaxAlertRulesPerProj,
		MaxWebhookSubsPerProj:   p.MaxWebhookSubsPerProj,
		MaxLogDrainsPerOrg:      p.MaxLogDrainsPerOrg,
		MaxAIModelCallsPerDay:   p.MaxAIModelCallsPerDay,
		AIAssistantBYOK:         p.AIAssistantBYOK,
		HasRBAC:                 p.HasRBAC,
		RBACLevel:               p.RBACLevel,
		HasAuditLogs:            p.HasAuditLogs,
		HasSSO:                  p.HasSSO,
		HasSLA:                  p.HasSLA,
		RequiresCreditCard:      p.RequiresCreditCard,
		OveragePerKRunsMicrousd: p.OveragePerKRunsMicrousd,
		SupportLevel:            p.SupportLevel,
		HasDedicatedCompute:     p.HasDedicatedCompute,
		HasStaticIPs:            p.HasStaticIPs,
		HasVPCPeering:           p.HasVPCPeering,
		HasSCIM:                 p.HasSCIM,
		HasDataResidency:        p.HasDataResidency,
		HasCustomRBAC:           p.HasCustomRBAC,
		HasReservedCapacity:     p.HasReservedCapacity,
		HasPriorityQueue:        p.HasPriorityQueue,
		HasIPAllowlisting:       p.HasIPAllowlisting,
		HasSessionManagement:    p.HasSessionManagement,
		HasSecretRotation:       p.HasSecretRotation,
		HasSIEMExport:           p.HasSIEMExport,
	}
}

// GetPlansOutput is the typed output for the list plans endpoint.
type GetPlansOutput struct {
	Body struct {
		Plans []planResponse `json:"plans"`
	}
}

// handleGetPlans returns all plan tier definitions with their limits and features.
func (s *Server) handleGetPlans(_ context.Context, _ *struct{}) (*GetPlansOutput, error) {
	tierOrder := []domain.PlanTier{
		domain.PlanFree,
		domain.PlanStarter,
		domain.PlanPro,
		domain.PlanScale,
		domain.PlanEnterprise,
	}

	plans := make([]planResponse, 0, len(tierOrder))
	for _, tier := range tierOrder {
		plans = append(plans, toPlanResponse(billing.Plans[tier]))
	}

	out := &GetPlansOutput{}
	out.Body.Plans = plans
	return out, nil
}
