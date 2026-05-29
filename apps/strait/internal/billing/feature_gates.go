package billing

import "strait/internal/domain"

// PlanFeatureGates are internal capability flags that gate feature access.
// Never exposed in any public API; never returned to customers.
type PlanFeatureGates struct {
	HasRBAC              bool
	RBACLevel            string // "" | "basic" | "advanced" | "custom"
	HasAuditLogs         bool
	HasSSO               bool
	HasSCIM              bool
	HasSLA               bool
	HasIPAllowlisting    bool
	HasSessionManagement bool
	HasSecretRotation    bool
	HasSIEMExport        bool
	HasDataResidency     bool
	HasDedicatedCompute  bool
	HasStaticIPs         bool
	HasVPCPeering        bool
	HasCustomRBAC        bool
	HasPriorityQueue     bool
	AIAssistantBYOK      bool
	SupportLevel         string // "community" | "email_72h" | "priority_24h" | "priority_slack_8h" | "dedicated"
}

// GetPlanGates returns the feature gates for a tier; falls back to Free.
// The pricing catalog is the only active entitlement source.
func GetPlanGates(tier domain.PlanTier) PlanFeatureGates {
	limits := GetPlanLimits(tier)
	if tier != limits.PlanTier {
		limits = GetPlanLimits(domain.PlanFree)
	}
	return PlanFeatureGates{
		HasRBAC:              limits.HasRBAC,
		RBACLevel:            limits.RBACLevel,
		HasAuditLogs:         limits.HasAuditLogs,
		HasSSO:               limits.HasSSO,
		HasSCIM:              limits.HasSCIM,
		HasSLA:               limits.HasSLA,
		HasIPAllowlisting:    limits.HasIPAllowlisting,
		HasSessionManagement: limits.HasSessionManagement,
		HasSecretRotation:    limits.HasSecretRotation,
		HasSIEMExport:        limits.HasSIEMExport,
		HasDataResidency:     limits.HasDataResidency,
		HasDedicatedCompute:  limits.HasDedicatedCompute,
		HasStaticIPs:         limits.HasStaticIPs,
		HasVPCPeering:        limits.HasVPCPeering,
		HasCustomRBAC:        limits.HasCustomRBAC,
		HasPriorityQueue:     limits.HasPriorityQueue,
		AIAssistantBYOK:      limits.AIAssistantBYOK,
		SupportLevel:         limits.SupportLevel,
	}
}
