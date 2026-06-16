package api

import (
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/billing"
	"strait/internal/domain"
)

// checkHTTPModeAllowed verifies that HTTP execution mode is allowed for the org's plan.
// Returns nil if allowed, or a 400 error if the plan doesn't support HTTP mode.
func (s *Server) checkHTTPModeAllowed(ctx context.Context, mode domain.ExecutionMode, projectID string) error {
	if mode != domain.ExecutionModeHTTP {
		return nil
	}
	if !s.edition.RequiresHTTPModeGating() {
		return nil
	}
	if s.billingEnforcer == nil {
		if s.allowsUngatedCloudDevelopment() {
			return nil
		}
		return planGateUnavailable("http_mode_enforcer", errors.New("billing enforcer not configured"))
	}

	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		if err != nil {
			return planGateUnavailable("http_mode_org_lookup", err)
		}
		return nil
	}

	limits, err := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		return planGateUnavailable("http_mode_plan_lookup", err)
	}

	if !limits.AllowsHTTPMode {
		billing.RecordHTTPModeGateRejected(ctx, string(limits.PlanTier), "job_create")
		return huma.Error400BadRequest("HTTP execution mode is unavailable for this organization. Contact support if this persists.")
	}
	return nil
}
