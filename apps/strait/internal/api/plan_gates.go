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

// checkEnvironmentLimit verifies that the project has not exceeded its
// plan's MaxEnvironments. Counts environments via the store.
func (s *Server) checkEnvironmentLimit(ctx context.Context, projectID string) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if limits.MaxEnvironments <= 0 {
		return nil // unlimited or not enforced
	}

	count, err := s.store.CountEnvironmentsByProject(ctx, projectID)
	if err != nil {
		return nil //nolint:nilerr // fail open: billing unavailable should not block environment creation
	}

	if count >= limits.MaxEnvironments {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d environments (you have %d). Upgrade at /settings/billing",
				limits.DisplayName, limits.MaxEnvironments, count),
		)
	}

	return nil
}

// checkScheduleLimit verifies that the org has not exceeded its plan's
// MaxScheduledJobs when adding a new cron job.
func (s *Server) checkScheduleLimit(ctx context.Context, projectID string, cronExpr string) error {
	if cronExpr == "" {
		return nil // not a scheduled job
	}

	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if limits.MaxScheduledJobs == -1 {
		return nil // unlimited
	}

	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		return nil //nolint:nilerr // fail open
	}

	count, err := s.store.CountCronJobsByOrg(ctx, orgID)
	if err != nil {
		return nil //nolint:nilerr // fail open: billing unavailable should not block schedule creation
	}

	if count >= limits.MaxScheduledJobs {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d scheduled jobs (you have %d). Upgrade at /settings/billing",
				limits.DisplayName, limits.MaxScheduledJobs, count),
		)
	}

	return nil
}

// checkWebhookEndpointLimit verifies that the project has not exceeded its
// plan's MaxWebhookEndpoints.
func (s *Server) checkWebhookEndpointLimit(ctx context.Context, projectID string) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if limits.MaxWebhookEndpoints == -1 {
		return nil // unlimited
	}

	if limits.MaxWebhookEndpoints == 0 {
		return huma.Error400BadRequest(
			fmt.Sprintf("Webhooks are not available on the %s plan. Upgrade at /settings/billing", limits.DisplayName),
		)
	}

	count, err := s.store.CountWebhookSubscriptionsByProject(ctx, projectID)
	if err != nil {
		return nil //nolint:nilerr // fail open: billing unavailable should not block webhook creation
	}

	if count >= limits.MaxWebhookEndpoints {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d webhook endpoints (you have %d). Upgrade at /settings/billing",
				limits.DisplayName, limits.MaxWebhookEndpoints, count),
		)
	}

	return nil
}

// basicWebhookEvents is the set of events available on the "basic" webhook tier.
var basicWebhookEvents = map[string]bool{
	"run.completed": true,
	"run.failed":    true,
}

// checkWebhookEventTypes verifies that the requested event types are allowed
// on the project's plan WebhookEventLevel.
func (s *Server) checkWebhookEventTypes(ctx context.Context, projectID string, eventTypes []string) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	switch limits.WebhookEventLevel {
	case "none":
		return huma.Error400BadRequest(
			fmt.Sprintf("Webhooks are not available on the %s plan. Upgrade at /settings/billing", limits.DisplayName),
		)
	case "basic":
		for _, et := range eventTypes {
			if !basicWebhookEvents[et] {
				return huma.Error400BadRequest(
					fmt.Sprintf("Event type %q requires the Pro plan or higher. Your plan only supports basic events (run.completed, run.failed). Upgrade at /settings/billing", et),
				)
			}
		}
	case "all", "all_custom":
		// all events allowed
	}

	return nil
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
