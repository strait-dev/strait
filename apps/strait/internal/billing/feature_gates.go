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

// PlanGates is the internal feature-gate matrix.
var PlanGates = map[domain.PlanTier]PlanFeatureGates{
	domain.PlanFree: {
		SupportLevel: "community",
	},
	domain.PlanStarter: {
		HasRBAC:      true,
		RBACLevel:    "basic",
		SupportLevel: "email_72h",
	},
	domain.PlanPro: {
		HasRBAC:         true,
		RBACLevel:       "advanced",
		HasAuditLogs:    true,
		AIAssistantBYOK: true,
		SupportLevel:    "priority_24h",
	},
	domain.PlanScale: {
		HasRBAC:              true,
		RBACLevel:            "advanced",
		HasAuditLogs:         true,
		HasSSO:               true,
		HasSCIM:              true,
		HasIPAllowlisting:    true,
		HasSessionManagement: true,
		AIAssistantBYOK:      true,
		HasPriorityQueue:     true,
		SupportLevel:         "priority_slack_8h",
	},
	domain.PlanBusiness: {
		HasRBAC:              true,
		RBACLevel:            "advanced",
		HasAuditLogs:         true,
		HasSSO:               true,
		HasSCIM:              true,
		HasIPAllowlisting:    true,
		HasSessionManagement: true,
		HasSecretRotation:    true,
		HasSIEMExport:        true,
		AIAssistantBYOK:      true,
		HasPriorityQueue:     true,
		HasSLA:               true,
		SupportLevel:         "priority_slack_8h",
	},
	domain.PlanEnterprise: {
		HasRBAC:              true,
		RBACLevel:            "custom",
		HasAuditLogs:         true,
		HasSSO:               true,
		HasSCIM:              true,
		HasIPAllowlisting:    true,
		HasSessionManagement: true,
		HasSecretRotation:    true,
		HasSIEMExport:        true,
		HasDataResidency:     true,
		HasDedicatedCompute:  true,
		HasStaticIPs:         true,
		HasVPCPeering:        true,
		HasCustomRBAC:        true,
		HasPriorityQueue:     true,
		HasSLA:               true,
		AIAssistantBYOK:      true,
		SupportLevel:         "dedicated",
	},
}

// GetPlanGates returns the feature gates for a tier; falls back to Free.
func GetPlanGates(tier domain.PlanTier) PlanFeatureGates {
	if g, ok := PlanGates[tier]; ok {
		return g
	}
	return PlanGates[domain.PlanFree]
}
