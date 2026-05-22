package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/robfig/cron/v3"

	"strait/internal/billing"
	"strait/internal/clickhouse"
	"strait/internal/domain"
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

// dispatchWorkflowRegistrationRejected emits a workflow.registration_rejected
// outbound billing webhook. Fire-and-forget: errors are swallowed inside
// Enforcer.DispatchBilling so a webhook delivery failure never converts a
// plan-gate 4xx into a 5xx for the caller.
func (s *Server) dispatchWorkflowRegistrationRejected(ctx context.Context, projectID, reason string, requestedValue, capValue any) {
	if s.billingEnforcer == nil {
		return
	}
	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		return
	}
	limits, err := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		return
	}
	detail := map[string]any{
		"reason":          reason,
		"project_id":      projectID,
		"requested_value": requestedValue,
		"cap":             capValue,
	}
	s.billingEnforcer.DispatchBilling(ctx, orgID, limits.PlanTier, domain.WebhookEventWorkflowRegistrationRejected, detail)
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
	billing.RecordFeatureGateRejected(ctx, string(feature), string(limits.PlanTier))

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
		s.dispatchWorkflowRegistrationRejected(ctx, projectID, "workflow_step_limit", stepCount, limits.MaxWorkflowDAGSteps)
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
		s.dispatchWorkflowRegistrationRejected(ctx, projectID, "cron_min_interval", int(minGap.Seconds()), limits.CronMinIntervalSec)
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

	s.dispatchWorkflowRegistrationRejected(ctx, projectID, "cron_overlap_policy", policy, "allow")
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
		s.dispatchWorkflowRegistrationRejected(ctx, projectID, "schedule_limit", count, limits.MaxScheduledJobs)
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

// checkLogDrainLimit verifies that the org has not exceeded its plan's
// MaxLogDrainsPerOrg. Counts across ALL projects to match downgrade cleanup.
func (s *Server) checkLogDrainLimit(ctx context.Context, projectID string) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if limits.MaxLogDrainsPerOrg == -1 {
		return nil // unlimited
	}

	if limits.MaxLogDrainsPerOrg == 0 {
		return huma.Error400BadRequest(
			fmt.Sprintf("Log drains are not available on the %s plan. Upgrade at /settings/billing", limits.DisplayName),
		)
	}

	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil || orgID == "" {
		return nil //nolint:nilerr // fail open: billing unavailable should not block creation
	}

	count, err := s.store.CountLogDrainsByOrg(ctx, orgID)
	if err != nil {
		return nil //nolint:nilerr // fail open: count failure should not block creation
	}

	if count >= limits.MaxLogDrainsPerOrg {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d log drains (you have %d). Upgrade at /settings/billing",
				limits.DisplayName, limits.MaxLogDrainsPerOrg, count),
		)
	}

	return nil
}

// checkNotificationChannelLimit verifies that the project has not exceeded
// its plan's MaxNotificationChannels. Counted per-project to match the
// channel's project-scoped storage model.
func (s *Server) checkNotificationChannelLimit(ctx context.Context, projectID string) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if limits.MaxNotificationChannels == -1 {
		return nil // unlimited
	}

	if limits.MaxNotificationChannels == 0 {
		return huma.Error400BadRequest(
			fmt.Sprintf("Notification channels are not available on the %s plan. Upgrade at /settings/billing", limits.DisplayName),
		)
	}

	count, err := s.store.CountNotificationChannelsByProject(ctx, projectID)
	if err != nil {
		return nil //nolint:nilerr // fail open: count failure should not block creation
	}

	if count >= limits.MaxNotificationChannels {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d notification channels per project (you have %d). Upgrade at /settings/billing",
				limits.DisplayName, limits.MaxNotificationChannels, count),
		)
	}

	return nil
}

// checkAlertRuleLimit verifies that the project has not exceeded its plan's
// MaxAlertRulesPerProj. The alert rules HTTP handler does not yet exist; the
// gate is wired here so that when the handler lands it can adopt the same
// per-tier cap pattern used by webhooks, log drains, and channels.
//
//nolint:unparam // projectID is always "proj-1" in tests until the handler lands; this is wired in advance.
func (s *Server) checkAlertRuleLimit(ctx context.Context, projectID string, currentCount int) error {
	limits := s.getOrgPlanLimits(ctx, projectID)
	if limits == nil {
		return nil // fail open
	}

	if limits.MaxAlertRulesPerProj == -1 {
		return nil // unlimited
	}

	if limits.MaxAlertRulesPerProj == 0 {
		return huma.Error400BadRequest(
			fmt.Sprintf("Alert rules are not available on the %s plan. Upgrade at /settings/billing", limits.DisplayName),
		)
	}

	if currentCount >= limits.MaxAlertRulesPerProj {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d alert rules per project (you have %d). Upgrade at /settings/billing",
				limits.DisplayName, limits.MaxAlertRulesPerProj, currentCount),
		)
	}

	return nil
}

// checkDailyAIModelCallLimit gates SDK AI usage reports against the org's
// MaxAIModelCallsPerDay quota. The runID is resolved to a project, then to an
// org, then the enforcer's Redis-backed atomic INCR check fires. Free-tier
// orgs are hard-rejected with 429; paid plans allow overage (logged for
// metering, never blocked).
func (s *Server) checkDailyAIModelCallLimit(ctx context.Context, runID string) error {
	if s.billingEnforcer == nil {
		return nil // fail open: community / unconfigured
	}
	if !s.edition.RequiresHTTPModeGating() {
		return nil
	}
	run, err := s.store.GetRun(ctx, runID)
	if err != nil || run == nil {
		return nil //nolint:nilerr // fail open: run lookup failures shouldn't drop usage telemetry
	}
	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, run.ProjectID)
	if err != nil || orgID == "" {
		return nil //nolint:nilerr // fail open
	}
	if err := s.billingEnforcer.CheckDailyAIModelCallLimit(ctx, orgID); err != nil {
		var le *billing.LimitError
		if errors.As(err, &le) {
			return &typedAPIError{status: 429, apiError: APIError{Code: le.Code, Message: le.Message, Details: []string{fmt.Sprintf("limit=%d current=%d", le.Limit, le.CurrentUsage)}}}
		}
		return nil
	}
	return nil
}

// checkRunTTLLimit caps a job's RunTTLSecs to the org's plan retention
// window. The cap is identical for every tier-bounded plan: a run row may
// not outlive the period the platform agrees to retain it.
//
// A zero TTL means "use the platform default" — no cap applies. A
// retention of -1 in OrgPlanLimits means "unlimited" — no cap applies.
// Otherwise the request must satisfy ttlSecs <= retentionDays * 86400.
func (s *Server) checkRunTTLLimit(ctx context.Context, projectID string, ttlSecs int) error {
	if ttlSecs <= 0 {
		return nil
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
	if limits.RetentionDays <= 0 {
		return nil // unlimited or unset
	}
	maxTTL := limits.RetentionDays * 86400
	if ttlSecs > maxTTL {
		s.dispatchWorkflowRegistrationRejected(ctx, projectID, "run_ttl_limit", ttlSecs, maxTTL)
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan retains runs for %d days (max run_ttl_secs = %d). Requested %d. Upgrade at /settings/billing",
				limits.DisplayName, limits.RetentionDays, maxTTL, ttlSecs),
		)
	}
	return nil
}

// checkPerJobConcurrencyLimit caps a job's MaxConcurrency and
// MaxConcurrencyPerKey settings to the org's plan MaxConcurrentRuns. The
// per-job budget cannot exceed the org-wide concurrent-run cap — otherwise
// a Free org could pin a single job to a Pro-tier concurrency value.
//
// Zero means "unset" (engine default applies); the gate ignores zero. A
// MaxConcurrentRuns of -1 means unlimited and skips the cap entirely.
func (s *Server) checkPerJobConcurrencyLimit(ctx context.Context, projectID string, maxConcurrency, maxConcurrencyPerKey int) error {
	if maxConcurrency <= 0 && maxConcurrencyPerKey <= 0 {
		return nil
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
	if limits.MaxConcurrentRuns < 0 {
		return nil
	}
	if maxConcurrency > limits.MaxConcurrentRuns {
		s.dispatchWorkflowRegistrationRejected(ctx, projectID, "per_job_concurrency", maxConcurrency, limits.MaxConcurrentRuns)
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d concurrent runs (max max_concurrency = %d). Requested %d. Upgrade at /settings/billing",
				limits.DisplayName, limits.MaxConcurrentRuns, limits.MaxConcurrentRuns, maxConcurrency),
		)
	}
	if maxConcurrencyPerKey > limits.MaxConcurrentRuns {
		s.dispatchWorkflowRegistrationRejected(ctx, projectID, "per_job_concurrency_per_key", maxConcurrencyPerKey, limits.MaxConcurrentRuns)
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d concurrent runs (max max_concurrency_per_key = %d). Requested %d. Upgrade at /settings/billing",
				limits.DisplayName, limits.MaxConcurrentRuns, limits.MaxConcurrentRuns, maxConcurrencyPerKey),
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
				s.dispatchWorkflowRegistrationRejected(ctx, projectID, "approval_gates_unavailable", "approval", "pro_required")
				return huma.Error400BadRequest(
					fmt.Sprintf("Approval gates require the Pro plan or higher. Your plan: %s. Upgrade at /settings/billing", limits.DisplayName),
				)
			}
		case "sub_workflow":
			if !limits.HasSubWorkflows {
				s.dispatchWorkflowRegistrationRejected(ctx, projectID, "sub_workflows_unavailable", "sub_workflow", "pro_required")
				return huma.Error400BadRequest(
					fmt.Sprintf("Sub-workflows require the Pro plan or higher. Your plan: %s. Upgrade at /settings/billing", limits.DisplayName),
				)
			}
		}
	}

	return nil
}
