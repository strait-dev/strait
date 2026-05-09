package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/robfig/cron/v3"

	"strait/internal/billing"
	"strait/internal/clickhouse"
)

// cronMinIntervalParser matches the standard 5-field parser used at validation
// time so the gate sees the same schedule the engine will run.
var cronMinIntervalParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// cronMinIntervalSampleCount controls how many consecutive firings we walk to
// estimate the minimum gap. Schedules like "0 9 * * MON,FRI" have variable
// gaps; sampling 50 firings catches the smallest one for any realistic plan.
const cronMinIntervalSampleCount = 50

// staticRegistry is a singleton PlanRegistry used by all plan gate checks.
var staticRegistry = billing.NewStaticRegistry()

// recordBillingEvent enqueues a billing analytics event to ClickHouse.
// No-op if the exporter is nil (self-hosted or analytics disabled).
func (s *Server) recordBillingEvent(ctx context.Context, projectID, eventType, feature, planTier string) {
	if s.chExporter == nil {
		return
	}
	var orgID string
	if s.billingEnforcer != nil {
		orgID, _ = s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	}
	s.chExporter.Enqueue(clickhouse.BillingEventRecord{
		Timestamp: time.Now(),
		OrgID:     orgID,
		ProjectID: projectID,
		EventType: eventType,
		Feature:   feature,
		PlanTier:  planTier,
	})
}

// getOrgPlanLimits resolves the org's plan limits from a project ID.
// Returns nil limits (and no error) when billing is unavailable or not
// configured -- callers should treat nil as "no enforcement" (fail open).
func (s *Server) getOrgPlanLimits(ctx context.Context, projectID string) *billing.OrgPlanLimits {
	if !s.edition.RequiresHTTPModeGating() || s.billingEnforcer == nil {
		return nil
	}

	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		slog.Warn("plan gate: failed to resolve org for project", "project_id", projectID, "error", err)
		return nil
	}

	limits, err := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		slog.Warn("plan gate: failed to get org plan limits", "org_id", orgID, "error", err)
		return nil
	}

	return &limits
}

// checkFeatureAllowed checks whether a plan-gated feature is available for
// the given project's org. Returns nil if allowed or if billing is unavailable
// (fail open). Returns a 403 error with structured metadata if blocked.
func (s *Server) checkFeatureAllowed(ctx context.Context, projectID string, feature billing.Feature, featureName string) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if staticRegistry.AllowsFeature(limits.PlanTier, feature) {
		return nil
	}

	s.recordBillingEvent(ctx, projectID, "gate_rejected", string(feature), string(limits.PlanTier))

	requiredPlan := staticRegistry.RequiredPlanForFeature(feature)

	return huma.Error403Forbidden(
		fmt.Sprintf("%s is not available on the %s plan. Upgrade to %s or higher.",
			featureName, limits.DisplayName, requiredPlan),
		&huma.ErrorDetail{
			Location: "billing",
			Message:  "feature_not_available",
			Value: map[string]string{
				"feature":       string(feature),
				"current_plan":  string(limits.PlanTier),
				"required_plan": string(requiredPlan),
			},
		},
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

// checkCronMinInterval rejects schedules that fire more frequently than the
// plan's CronMinIntervalSec. Free=300s, Starter=60s, Pro=30s; Scale and above
// use 0/1s minimums that the 5-field cron format cannot violate, so this gate
// is effectively a Free/Starter/Pro guard. Empty cronExpr is a no-op so the
// caller can hand off the user's input verbatim from create/update requests.
func (s *Server) checkCronMinInterval(ctx context.Context, projectID, cronExpr string) error {
	if cronExpr == "" {
		return nil
	}

	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}
	if limits.CronMinIntervalSec <= 0 {
		return nil // sub-second tier, nothing to enforce here
	}

	schedule, err := cronMinIntervalParser.Parse(cronExpr)
	if err != nil {
		return nil //nolint:nilerr // request-level validation already rejects malformed expressions
	}

	// Walk a window of upcoming firings and pick the smallest gap. We seed
	// from a fixed UTC instant to keep the result deterministic; the schedule
	// fields all repeat over a year so a small sample catches the worst case.
	ref := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	prev := schedule.Next(ref)
	if prev.IsZero() {
		return nil
	}
	minGap := time.Duration(0)
	for range cronMinIntervalSampleCount {
		next := schedule.Next(prev)
		if next.IsZero() {
			break
		}
		gap := next.Sub(prev)
		if minGap == 0 || gap < minGap {
			minGap = gap
		}
		prev = next
	}
	if minGap == 0 {
		return nil
	}

	minRequired := time.Duration(limits.CronMinIntervalSec) * time.Second
	if minGap < minRequired {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan requires at least %ds between cron firings (this schedule fires every %ds). Upgrade at /settings/billing",
				limits.DisplayName, limits.CronMinIntervalSec, int(minGap.Seconds())),
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

// checkEnvironmentLimit verifies that the org has not exceeded its
// plan's MaxEnvironments. Counts environments across ALL projects in the org
// to match the downgrade cleanup logic (DeactivateExcessEnvironments).
func (s *Server) checkEnvironmentLimit(ctx context.Context, projectID string) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if limits.MaxEnvironments <= 0 {
		return nil // unlimited or not enforced
	}

	// Count org-wide to match downgrade enforcement scope.
	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		return nil //nolint:nilerr // fail open
	}
	count, err := s.store.CountEnvironmentsByOrg(ctx, orgID)
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

	if !s.edition.RequiresHTTPModeGating() || s.billingEnforcer == nil {
		return nil
	}

	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		return nil //nolint:nilerr // fail open
	}

	limits, limErr := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if limErr != nil {
		return nil //nolint:nilerr // fail open
	}
	if limits.MaxScheduledJobs == -1 {
		return nil // unlimited
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

// checkWebhookEndpointLimit verifies that the org has not exceeded its
// plan's MaxWebhookEndpoints. Counts across ALL projects to match downgrade cleanup.
func (s *Server) checkWebhookEndpointLimit(ctx context.Context, projectID string) error {
	orgID, maxEndpoints, displayName, err := s.resolveWebhookEndpointCreateLimit(ctx, projectID)
	if err != nil {
		return err
	}
	if orgID == "" || maxEndpoints < 0 {
		return nil
	}

	count, err := s.store.CountWebhookSubscriptionsByOrg(ctx, orgID)
	if err != nil {
		return nil //nolint:nilerr // fail open: billing unavailable should not block webhook creation
	}

	if count >= maxEndpoints {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d webhook endpoints (you have %d). Upgrade at /settings/billing",
				displayName, maxEndpoints, count),
		)
	}

	return nil
}

func (s *Server) resolveWebhookEndpointCreateLimit(ctx context.Context, projectID string) (string, int, string, error) {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return "", -1, "", nil // fail open
	}

	if limits.MaxWebhookEndpoints == -1 {
		return "", -1, limits.DisplayName, nil // unlimited
	}

	if limits.MaxWebhookEndpoints == 0 {
		return "", 0, limits.DisplayName, huma.Error400BadRequest(
			fmt.Sprintf("Webhooks are not available on the %s plan. Upgrade at /settings/billing", limits.DisplayName),
		)
	}

	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		return "", -1, limits.DisplayName, nil //nolint:nilerr // fail open
	}

	return orgID, limits.MaxWebhookEndpoints, limits.DisplayName, nil
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
