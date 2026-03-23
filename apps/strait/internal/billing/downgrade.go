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
	projects, err := store.ListProjectsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing projects for downgrade preview: %w", err)
	}

	// Determine effective date from subscription period end.
	now := time.Now().UTC()
	effectiveDate := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC)
	if sub.CurrentPeriodEnd != nil {
		effectiveDate = *sub.CurrentPeriodEnd
	}

	impact := &DowngradeImpact{
		TargetTier:    string(targetTier),
		EffectiveDate: effectiveDate.Format("2006-01-02"),
	}

	// Projects
	impact.Impacts = append(impact.Impacts, buildImpact(
		"projects",
		int64(len(projects)),
		int64(targetLimits.MaxProjectsPerOrg),
	))

	// Members per org
	impact.Impacts = append(impact.Impacts, buildImpact(
		"members_per_org",
		int64(currentLimits.MaxMembersPerOrg),
		int64(targetLimits.MaxMembersPerOrg),
	))

	// Runs per day
	impact.Impacts = append(impact.Impacts, buildImpact(
		"runs_per_day",
		currentLimits.MaxRunsPerDay,
		targetLimits.MaxRunsPerDay,
	))

	// Concurrent runs
	impact.Impacts = append(impact.Impacts, buildImpact(
		"concurrent_runs",
		int64(currentLimits.MaxConcurrentRuns),
		int64(targetLimits.MaxConcurrentRuns),
	))

	// Compute credit
	impact.Impacts = append(impact.Impacts, buildImpact(
		"compute_credit",
		currentLimits.ComputeCreditMicrousd,
		targetLimits.ComputeCreditMicrousd,
	))

	// Retention days
	impact.Impacts = append(impact.Impacts, buildImpact(
		"retention_days",
		int64(currentLimits.RetentionDays),
		int64(targetLimits.RetentionDays),
	))

	// Regions
	currentRegions := len(currentLimits.AllowedRegions)
	if currentRegions == 0 {
		currentRegions = 25 // nil means all
	}
	targetRegions := len(targetLimits.AllowedRegions)
	if targetRegions == 0 {
		targetRegions = 25
	}
	impact.Impacts = append(impact.Impacts, buildImpact(
		"regions",
		int64(currentRegions),
		int64(targetRegions),
	))

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
