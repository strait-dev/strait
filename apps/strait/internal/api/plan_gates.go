package api

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"strait/internal/billing"
)

// getOrgPlanLimits resolves the org's plan limits from a project ID.
// Returns nil limits (and no error) when billing is unavailable or not
// configured -- callers should treat nil as "no enforcement" (fail open).
func (s *Server) getOrgPlanLimits(ctx context.Context, projectID string) *billing.OrgPlanLimits {
	if !s.edition.RequiresHTTPModeGating() || s.billingEnforcer == nil {
		return nil
	}

	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		return nil
	}

	limits, err := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		return nil
	}

	return &limits
}

// checkFeatureAllowed checks whether a plan-gated feature is available for
// the given project's org. Returns nil if allowed or if billing is unavailable
// (fail open). Returns a 400 error with an upgrade prompt if blocked.
func (s *Server) checkFeatureAllowed(ctx context.Context, projectID string, feature billing.Feature, featureName string) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	reg := billing.NewStaticRegistry()
	if reg.AllowsFeature(limits.PlanTier, feature) {
		return nil
	}

	return huma.Error400BadRequest(
		fmt.Sprintf("%s requires a higher plan. Upgrade at /settings/billing", featureName),
	)
}

// checkPresetAllowed verifies that the requested machine preset is available
// on the project's org plan. Returns nil if allowed or if billing is unavailable.
func (s *Server) checkPresetAllowed(ctx context.Context, projectID, preset string) error {
	if preset == "" {
		return nil
	}

	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if limits.IsPresetAllowed(preset) {
		return nil
	}

	return huma.Error400BadRequest(
		fmt.Sprintf("Machine preset %q is not available on the %s plan. Upgrade at /settings/billing", preset, limits.DisplayName),
	)
}

// checkWorkflowStepLimit verifies that the number of steps does not exceed
// the plan's MaxWorkflowDAGSteps. Returns nil if within limits.
func (s *Server) checkWorkflowStepLimit(ctx context.Context, projectID string, stepCount int) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if limits.MaxWorkflowDAGSteps == -1 {
		return nil // unlimited
	}

	if stepCount > limits.MaxWorkflowDAGSteps {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d workflow steps (you have %d). Upgrade at /settings/billing",
				limits.DisplayName, limits.MaxWorkflowDAGSteps, stepCount),
		)
	}

	return nil
}

// checkCronOverlapPolicy verifies that the requested overlap policy is
// allowed on the plan. Free tier only allows "allow".
func (s *Server) checkCronOverlapPolicy(ctx context.Context, projectID, policy string) error {
	if policy == "" || policy == "allow" {
		return nil // "allow" is available on all plans
	}

	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if limits.AllCronOverlapPolicies {
		return nil
	}

	return huma.Error400BadRequest(
		fmt.Sprintf("Cron overlap policy %q requires the Starter plan or higher. Upgrade at /settings/billing", policy),
	)
}

// checkJobChainingAllowed verifies that job chaining (on_complete_trigger_job)
// is allowed on the project's plan.
func (s *Server) checkJobChainingAllowed(ctx context.Context, projectID string, triggerJob, triggerWorkflow string) error {
	if triggerJob == "" && triggerWorkflow == "" {
		return nil
	}

	return s.checkFeatureAllowed(ctx, projectID, billing.FeatureJobChaining, "Job chaining")
}

// checkWorkflowStepFeatures verifies that step types used in a workflow are
// allowed on the project's plan (approval gates require Pro+, sub-workflows
// require Pro+).
func (s *Server) checkWorkflowStepFeatures(ctx context.Context, projectID string, steps []workflowStepRequest) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	for _, step := range steps {
		switch step.StepType { //nolint:exhaustive // only gating approval and sub_workflow types
		case "approval":
			if !limits.HasApprovalGates {
				return huma.Error400BadRequest(
					fmt.Sprintf("Approval gates require the Pro plan or higher. Your plan: %s. Upgrade at /settings/billing", limits.DisplayName),
				)
			}
		case "sub_workflow":
			if !limits.HasSubWorkflows {
				return huma.Error400BadRequest(
					fmt.Sprintf("Sub-workflows require the Pro plan or higher. Your plan: %s. Upgrade at /settings/billing", limits.DisplayName),
				)
			}
		}
	}

	return nil
}
