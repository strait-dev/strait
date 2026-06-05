package billing

import (
	"context"
	"fmt"
	"time"

	"strait/internal/domain"
)

// ResourceAction describes what happens to a resource on downgrade.
type ResourceAction string

const (
	ResourceActionOK     ResourceAction = "ok"
	ResourceActionReduce ResourceAction = "reduce"
	ResourceActionRemove ResourceAction = "remove"
)

// ResourceImpact describes the impact on a single resource when downgrading.
type ResourceImpact struct {
	Resource string         `json:"resource"`
	Current  int64          `json:"current"`
	Limit    int64          `json:"limit"`
	Action   ResourceAction `json:"action"`
}

// DowngradeImpact summarizes the effects of downgrading to a lower plan tier.
type DowngradeImpact struct {
	TargetTier    string           `json:"target_tier"`
	EffectiveDate string           `json:"effective_date"`
	Impacts       []ResourceImpact `json:"impacts"`
	ManualActions []ResourceImpact `json:"manual_actions"`
	AutoDisabled  []ResourceImpact `json:"auto_disabled"`
}

// PreviewDowngrade compares the org's current resource usage against the limits
// of the target tier and returns a summary of what would be affected.
func PreviewDowngrade(ctx context.Context, store Store, orgID string, targetTier domain.PlanTier) (*DowngradeImpact, error) {
	targetLimits := GetPlanLimits(targetTier)

	// Get current subscription to determine current tier.
	sub, err := store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("getting org subscription for downgrade preview: %w", err)
	}

	currentLimits := GetPlanLimits(domain.PlanTier(sub.PlanTier))

	// Get current project count.
	projectCount, err := store.CountProjectsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("counting projects for downgrade preview: %w", err)
	}

	// Determine effective date from subscription period end.
	now := time.Now().UTC()
	effectiveDate := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	if sub.CurrentPeriodEnd != nil {
		effectiveDate = *sub.CurrentPeriodEnd
	}

	memberCount, err := store.CountMembersByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("counting members for downgrade preview: %w", err)
	}
	executingRuns, err := store.CountExecutingRunsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("counting executing runs for downgrade preview: %w", err)
	}
	periodStart, periodEnd := usagePeriodWindow(now, domain.PlanTier(sub.PlanTier), sub)
	usage, err := store.GetOrgUsageForPeriod(ctx, orgID, periodStart, periodEnd)
	if err != nil {
		return nil, fmt.Errorf("getting usage for downgrade preview: %w", err)
	}
	var runsThisPeriod int64
	for _, rec := range usage {
		runsThisPeriod += rec.RunsCount
	}

	impact := &DowngradeImpact{
		TargetTier:    string(targetTier),
		EffectiveDate: effectiveDate.Format("2006-01-02"),
	}

	// Projects
	impact.Impacts = append(impact.Impacts, buildImpact(
		"projects",
		int64(projectCount),
		int64(targetLimits.MaxProjectsPerOrg),
	))

	// Members per org
	impact.Impacts = append(impact.Impacts, buildImpact(
		"members_per_org",
		int64(memberCount),
		int64(targetLimits.MaxMembersPerOrg),
	))

	// Runs per month
	impact.Impacts = append(impact.Impacts, buildImpact(
		"runs_per_month",
		runsThisPeriod,
		int64(targetLimits.MaxRunsPerMonth),
	))

	// Concurrent runs
	impact.Impacts = append(impact.Impacts, buildImpact(
		"concurrent_runs",
		int64(executingRuns),
		int64(targetLimits.MaxConcurrentRuns),
	))

	// Retention days
	impact.Impacts = append(impact.Impacts, buildImpact(
		"retention_days",
		int64(currentLimits.RetentionDays),
		int64(targetLimits.RetentionDays),
	))

	// HTTP-mode jobs (losing HTTP mode on downgrade = jobs auto-paused).
	if currentLimits.AllowsHTTPMode && !targetLimits.AllowsHTTPMode {
		httpJobs, err := store.CountHTTPJobsByOrg(ctx, orgID)
		if err != nil {
			return nil, fmt.Errorf("counting HTTP jobs for downgrade preview: %w", err)
		}
		if httpJobs > 0 {
			impact.Impacts = append(impact.Impacts, ResourceImpact{
				Resource: "http_mode_jobs",
				Current:  int64(httpJobs),
				Limit:    0,
				Action:   ResourceActionRemove,
			})
		}
	}

	// Separate impacts into manual actions vs auto-disabled.
	impact.ManualActions, impact.AutoDisabled = AutoDisableResources(impact.Impacts)

	return impact, nil
}

// AutoDisableResources separates resource impacts into those requiring manual
// user action (projects, members) and those that can be auto-disabled
// (log drains, alert rules, webhooks, custom roles, etc.).
func AutoDisableResources(impacts []ResourceImpact) (manualActions []ResourceImpact, autoDisabled []ResourceImpact) {
	for _, impact := range impacts {
		if impact.Action == ResourceActionOK {
			continue
		}
		switch impact.Resource {
		case "projects", "members", "members_per_org":
			manualActions = append(manualActions, impact)
		default:
			autoDisabled = append(autoDisabled, impact)
		}
	}
	return manualActions, autoDisabled
}

func buildImpact(resource string, current, limit int64) ResourceImpact {
	action := ResourceActionOK
	// -1 means unlimited, so any limit is a reduction.
	if limit >= 0 && (current == -1 || current > limit) {
		if limit == 0 {
			action = ResourceActionRemove
		} else {
			action = ResourceActionReduce
		}
	}
	return ResourceImpact{
		Resource: resource,
		Current:  current,
		Limit:    limit,
		Action:   action,
	}
}
