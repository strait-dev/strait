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
	"strait/internal/store"
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

func planGateUnavailable(resource string, err error) error {
	slog.Error("plan gate: enforcement dependency unavailable", "resource", resource, "error", err)
	return huma.Error503ServiceUnavailable("billing enforcement unavailable, please retry")
}

func (s *Server) getProjectOrgIDForPlanGate(ctx context.Context, projectID, resource string) (string, error) {
	orgID, err := s.billingEnforcer.GetProjectOrgID(ctx, projectID)
	if err != nil {
		return "", planGateUnavailable(resource, err)
	}
	if orgID == "" {
		return "", planGateUnavailable(resource, errors.New("project org id is empty"))
	}
	return orgID, nil
}

type logDrainOrgLimitCreator interface {
	CreateLogDrainWithOrgLimit(ctx context.Context, drain *domain.LogDrain, orgID string, maxDrains int) error
}

type notificationChannelProjectLimitCreator interface {
	CreateNotificationChannelWithProjectLimit(ctx context.Context, ch *domain.NotificationChannel, maxChannels int) error
}

type environmentOrgLimitCreator interface {
	CreateEnvironmentWithOrgLimit(ctx context.Context, env *domain.Environment, orgID string, maxEnvironments int) error
}

type jobCronScheduleLimitCreator interface {
	CreateJobWithCronScheduleLimit(ctx context.Context, job *domain.Job, orgID string, maxSchedules int) error
}

type jobCronScheduleLimitUpdater interface {
	UpdateJobWithCronScheduleLimit(ctx context.Context, job *domain.Job, orgID string, maxSchedules int) error
}

type cronScheduleLimitEnforcer interface {
	EnforceCronScheduleLimit(ctx context.Context, orgID string, maxSchedules int) error
}

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

// getOrgPlanLimits resolves the org's plan limits from a project ID. A nil
// limits result with nil error means this edition/server is ungated. A non-nil
// error means the enforcement dependency is unavailable and the caller should
// reject the gated operation.
func (s *Server) getOrgPlanLimits(ctx context.Context, projectID string) (*billing.OrgPlanLimits, error) {
	if !s.edition.RequiresHTTPModeGating() {
		return nil, nil
	}
	if s.billingEnforcer == nil {
		return nil, planGateUnavailable("plan_gate_enforcer", errors.New("billing enforcer not configured"))
	}

	orgID, err := s.getProjectOrgIDForPlanGate(ctx, projectID, "plan_gate_org_lookup")
	if err != nil {
		return nil, err
	}

	limits, err := s.billingEnforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		return nil, planGateUnavailable("plan_gate_plan_lookup", err)
	}

	return &limits, nil
}

// checkFeatureAllowed checks whether a plan-gated feature is available for
// the given project's org. Returns nil if allowed or if the edition is
// ungated. Returns 503 if cloud enforcement is unavailable, or 403 with
// structured metadata if blocked.
func (s *Server) checkFeatureAllowed(ctx context.Context, projectID string, feature billing.Feature, featureName string) error {
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return err
	}
	if limits == nil {
		return nil
	}

	if staticRegistry.AllowsFeature(limits.PlanTier, feature) {
		return nil
	}

	s.recordBillingEvent(ctx, projectID, "gate_rejected", string(feature), string(limits.PlanTier))
	billing.RecordFeatureGateRejected(ctx, string(feature), string(limits.PlanTier))

	requiredPlan := staticRegistry.RequiredPlanForFeature(feature)
	if requiredPlan == "" {
		return huma.Error403Forbidden(
			fmt.Sprintf("%s is roadmap/contact-sales only at launch and is not available on self-serve plans.",
				featureName),
			&huma.ErrorDetail{
				Location: "billing",
				Message:  "feature_roadmap",
				Value: map[string]string{
					"feature":      string(feature),
					"current_plan": string(limits.PlanTier),
					"status":       "roadmap_contact_sales",
				},
			},
		)
	}

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

func rbacLevelRank(level string) int {
	switch level {
	case "basic":
		return 1
	case "full":
		return 2
	case "advanced":
		return 3
	default:
		return 0
	}
}

func displayRBACLevel(level string) string {
	if level == "" {
		return "no"
	}
	return level
}

func (s *Server) isRBACLevelAllowed(ctx context.Context, projectID, minLevel string) bool {
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		slog.Warn("rbac policy gate: plan lookup failed", "project_id", projectID, "error", err)
		return false
	}
	if limits == nil {
		return true
	}
	return rbacLevelRank(limits.RBACLevel) >= rbacLevelRank(minLevel)
}

// checkRBACLevel verifies that an RBAC mutation is available for the
// customer's RBAC tier. Basic RBAC covers built-in roles and member assignment;
// custom role mutation requires Full or higher; policy-based authorization
// requires Advanced.
func (s *Server) checkRBACLevel(ctx context.Context, projectID, minLevel, featureName string) error {
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return err
	}
	if limits == nil {
		return nil
	}
	if rbacLevelRank(limits.RBACLevel) >= rbacLevelRank(minLevel) {
		return nil
	}

	feature := "rbac_" + minLevel
	s.recordBillingEvent(ctx, projectID, "gate_rejected", feature, string(limits.PlanTier))
	billing.RecordFeatureGateRejected(ctx, feature, string(limits.PlanTier))

	return huma.Error403Forbidden(
		fmt.Sprintf("%s requires %s RBAC. Your %s plan includes %s RBAC. Upgrade at /settings/billing.",
			featureName, minLevel, limits.DisplayName, displayRBACLevel(limits.RBACLevel)),
		&huma.ErrorDetail{
			Location: "billing",
			Message:  "rbac_level_not_available",
			Value: map[string]string{
				"feature":        feature,
				"current_plan":   string(limits.PlanTier),
				"current_level":  displayRBACLevel(limits.RBACLevel),
				"required_level": minLevel,
			},
		},
	)
}

// checkWorkflowStepLimit verifies that the number of steps does not exceed
// the plan's MaxWorkflowDAGSteps. Returns nil if within limits.
func (s *Server) checkWorkflowStepLimit(ctx context.Context, projectID string, stepCount int) error {
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return err
	}
	if limits == nil {
		return nil
	}

	if limits.MaxWorkflowDAGSteps == -1 {
		return nil // Unlimited.
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
// plan's CronMinIntervalSec. Launch tiers set Free=300s, Starter=60s,
// Pro=30s, Scale=1s, and Business/Enterprise=0. Empty cronExpr is a no-op so
// the caller can hand off the user's input verbatim from create/update requests.
func (s *Server) checkCronMinInterval(ctx context.Context, projectID, cronExpr string) error {
	if cronExpr == "" {
		return nil
	}

	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return err
	}
	if limits == nil {
		return nil
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

	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return err
	}
	if limits == nil {
		return nil
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
	orgID, maxEnvironments, displayName, err := s.resolveEnvironmentCreateLimit(ctx, projectID)
	if err != nil {
		return err
	}
	if orgID == "" || maxEnvironments < 0 {
		return nil
	}

	count, err := s.store.CountEnvironmentsByOrg(ctx, orgID)
	if err != nil {
		return planGateUnavailable("environment_count", err)
	}

	if count >= maxEnvironments {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d environments (you have %d). Upgrade at /settings/billing",
				displayName, maxEnvironments, count),
		)
	}

	return nil
}

func (s *Server) resolveEnvironmentCreateLimit(ctx context.Context, projectID string) (string, int, string, error) {
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return "", -1, "", err
	}
	if limits == nil {
		return "", -1, "", nil
	}

	if limits.MaxEnvironments <= 0 {
		return "", -1, limits.DisplayName, nil // Unlimited or not enforced.
	}

	orgID, err := s.getProjectOrgIDForPlanGate(ctx, projectID, "environment_org_lookup")
	if err != nil {
		return "", -1, limits.DisplayName, err
	}

	return orgID, limits.MaxEnvironments, limits.DisplayName, nil
}

// checkScheduleLimit verifies that the org has not exceeded its plan's
// MaxScheduledJobs when adding a new cron job.
func (s *Server) checkScheduleLimit(ctx context.Context, projectID string, cronExpr string) error {
	orgID, maxSchedules, displayName, err := s.resolveScheduleCreateLimit(ctx, projectID, cronExpr)
	if err != nil {
		return err
	}
	if orgID == "" || maxSchedules < 0 {
		return nil
	}

	count, err := s.store.CountCronJobsByOrg(ctx, orgID)
	if err != nil {
		return planGateUnavailable("schedule_count", err)
	}

	if count >= maxSchedules {
		s.dispatchWorkflowRegistrationRejected(ctx, projectID, "schedule_limit", count, maxSchedules)
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d scheduled jobs (you have %d). Upgrade at /settings/billing",
				displayName, maxSchedules, count),
		)
	}

	return nil
}

func (s *Server) resolveScheduleCreateLimit(ctx context.Context, projectID string, cronExpr string) (string, int, string, error) {
	if cronExpr == "" {
		return "", -1, "", nil
	}
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return "", -1, "", err
	}
	if limits == nil {
		return "", -1, "", nil
	}
	if limits.MaxScheduledJobs == -1 {
		return "", -1, limits.DisplayName, nil // Unlimited.
	}

	orgID, err := s.getProjectOrgIDForPlanGate(ctx, projectID, "schedule_org_lookup")
	if err != nil {
		return "", -1, limits.DisplayName, err
	}

	return orgID, limits.MaxScheduledJobs, limits.DisplayName, nil
}

func (s *Server) enforceCronScheduleLimitForStore(ctx context.Context, apiStore APIStore, projectID, cronExpr string) error {
	orgID, maxSchedules, displayName, err := s.resolveScheduleCreateLimit(ctx, projectID, cronExpr)
	if err != nil {
		return err
	}
	if orgID == "" || maxSchedules < 0 {
		return nil
	}

	if enforcer, ok := apiStore.(cronScheduleLimitEnforcer); ok {
		err = enforcer.EnforceCronScheduleLimit(ctx, orgID, maxSchedules)
	} else {
		err = s.checkScheduleLimit(ctx, projectID, cronExpr)
	}
	if errors.Is(err, store.ErrCronScheduleLimitExceeded) {
		s.dispatchWorkflowRegistrationRejected(ctx, projectID, "schedule_limit", maxSchedules, maxSchedules)
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d scheduled jobs. Upgrade at /settings/billing", displayName, maxSchedules),
		)
	}
	return err
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
		return planGateUnavailable("webhook_endpoint_count", err)
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
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return "", -1, "", err
	}
	if limits == nil {
		return "", -1, "", nil
	}

	if limits.MaxWebhookEndpoints == -1 {
		return "", -1, limits.DisplayName, nil // Unlimited.
	}

	if limits.MaxWebhookEndpoints == 0 {
		return "", 0, limits.DisplayName, huma.Error400BadRequest(
			fmt.Sprintf("Webhooks are not available on the %s plan. Upgrade at /settings/billing", limits.DisplayName),
		)
	}

	orgID, err := s.getProjectOrgIDForPlanGate(ctx, projectID, "webhook_endpoint_org_lookup")
	if err != nil {
		return "", -1, limits.DisplayName, err
	}

	return orgID, limits.MaxWebhookEndpoints, limits.DisplayName, nil
}

func (s *Server) resolveWebhookProjectCreateLimit(ctx context.Context, projectID string) (int, error) {
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return -1, err
	}
	if limits == nil || limits.MaxWebhookSubsPerProj == -1 {
		return -1, nil
	}

	if limits.MaxWebhookSubsPerProj == 0 {
		return 0, huma.Error400BadRequest(
			fmt.Sprintf("Webhook subscriptions are not available on the %s plan. Upgrade at /settings/billing", limits.DisplayName),
		)
	}

	return limits.MaxWebhookSubsPerProj, nil
}

func (s *Server) checkWebhookProjectLimit(ctx context.Context, projectID string, maxSubscriptions int) error {
	if maxSubscriptions < 0 {
		return nil
	}
	count, err := s.store.CountWebhookSubscriptionsByProject(ctx, projectID)
	if err != nil {
		return planGateUnavailable("webhook_project_count", err)
	}
	if count >= maxSubscriptions {
		return huma.Error400BadRequest("webhook subscription limit exceeded")
	}
	return nil
}

// checkLogDrainLimit verifies that the org has not exceeded its plan's
// MaxLogDrainsPerOrg. Counts across ALL projects to match downgrade cleanup.
func (s *Server) checkLogDrainLimit(ctx context.Context, projectID string) error {
	orgID, maxDrains, displayName, err := s.resolveLogDrainCreateLimit(ctx, projectID)
	if err != nil {
		return err
	}
	if orgID == "" || maxDrains < 0 {
		return nil
	}

	count, err := s.store.CountLogDrainsByOrg(ctx, orgID)
	if err != nil {
		return planGateUnavailable("log_drain_count", err)
	}

	if count >= maxDrains {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d log drains (you have %d). Upgrade at /settings/billing",
				displayName, maxDrains, count),
		)
	}

	return nil
}

func (s *Server) resolveLogDrainCreateLimit(ctx context.Context, projectID string) (string, int, string, error) {
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return "", -1, "", err
	}
	if limits == nil {
		return "", -1, "", nil
	}

	if limits.MaxLogDrainsPerOrg == -1 {
		return "", -1, limits.DisplayName, nil // Unlimited.
	}

	if limits.MaxLogDrainsPerOrg == 0 {
		return "", 0, limits.DisplayName, huma.Error400BadRequest(
			fmt.Sprintf("Log drains are not available on the %s plan. Upgrade at /settings/billing", limits.DisplayName),
		)
	}

	orgID, err := s.getProjectOrgIDForPlanGate(ctx, projectID, "log_drain_org_lookup")
	if err != nil {
		return "", -1, limits.DisplayName, err
	}

	return orgID, limits.MaxLogDrainsPerOrg, limits.DisplayName, nil
}

// checkNotificationChannelLimit verifies that the project has not exceeded
// its plan's MaxNotificationChannels. Counted per-project to match the
// channel's project-scoped storage model.
func (s *Server) checkNotificationChannelLimit(ctx context.Context, projectID string) error {
	maxChannels, displayName, err := s.resolveNotificationChannelCreateLimit(ctx, projectID)
	if err != nil {
		return err
	}
	if maxChannels < 0 {
		return nil
	}

	count, err := s.store.CountNotificationChannelsByProject(ctx, projectID)
	if err != nil {
		return planGateUnavailable("notification_channel_count", err)
	}

	if count >= maxChannels {
		return huma.Error400BadRequest(
			fmt.Sprintf("Your %s plan allows %d notification channels per project (you have %d). Upgrade at /settings/billing",
				displayName, maxChannels, count),
		)
	}

	return nil
}

func (s *Server) resolveNotificationChannelCreateLimit(ctx context.Context, projectID string) (int, string, error) {
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return -1, "", err
	}
	if limits == nil {
		return -1, "", nil
	}

	if limits.MaxNotificationChannels == -1 {
		return -1, limits.DisplayName, nil // Unlimited.
	}

	if limits.MaxNotificationChannels == 0 {
		return 0, limits.DisplayName, huma.Error400BadRequest(
			fmt.Sprintf("Notification channels are not available on the %s plan. Upgrade at /settings/billing", limits.DisplayName),
		)
	}

	return limits.MaxNotificationChannels, limits.DisplayName, nil
}

// checkAlertRuleLimit verifies that the project has not exceeded its plan's
// MaxAlertRulesPerProj. The alert rules HTTP handler does not yet exist; the
// gate is wired here so that when the handler lands it can adopt the same
// per-tier cap pattern used by webhooks, log drains, and channels.
//
//nolint:unparam // projectID is always "proj-1" in tests until the handler lands; this is wired in advance.
func (s *Server) checkAlertRuleLimit(ctx context.Context, projectID string, currentCount int) error {
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return err
	}
	if limits == nil {
		return nil
	}

	if limits.MaxAlertRulesPerProj == -1 {
		return nil // Unlimited.
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
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return err
	}
	if limits == nil {
		return nil
	}
	if limits.RetentionDays <= 0 {
		return nil // Unlimited. or unset
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
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return err
	}
	if limits == nil {
		return nil
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
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return err
	}
	if limits == nil {
		return nil
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
	limits, err := s.getOrgPlanLimits(ctx, projectID)
	if err != nil {
		return err
	}
	if limits == nil {
		return nil
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
