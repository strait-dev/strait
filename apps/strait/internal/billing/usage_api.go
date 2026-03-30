package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"strait/internal/domain"
)

// CurrentUsageResponse is the response for GET /v1/usage/current.
type CurrentUsageResponse struct {
	OrgID                string          `json:"org_id"`
	Plan                 string          `json:"plan"`
	Period               PeriodInfo      `json:"period"`
	Usage                UsageDimensions `json:"usage"`
	IncludedCreditMicro  int64           `json:"included_credit_microusd"`
	PeriodSpendMicro     int64           `json:"period_spend_microusd"`
	OverageMicro         int64           `json:"overage_microusd"`
	CreditUsedPercent    float64         `json:"credit_used_percent"`
	CreditRemainingMicro int64           `json:"credit_remaining_microusd"`
	Alerts               []UsageAlert    `json:"alerts,omitempty"`
	PaymentStatus        string          `json:"payment_status,omitempty"`
	GracePeriodEnd       *string         `json:"grace_period_end,omitempty"`
	ActiveAddons         []AddonSummary  `json:"active_addons,omitempty"`
}

// AddonSummary represents an active add-on in the usage response.
type AddonSummary struct {
	Type     string `json:"type"`
	Quantity int    `json:"quantity"`
}

// PeriodInfo describes the billing period.
type PeriodInfo struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// UsageDimension is a single quota dimension with usage vs limit.
type UsageDimension struct {
	Used    int64   `json:"used"`
	Limit   int64   `json:"limit"`
	Percent float64 `json:"percent"`
	Display string  `json:"display,omitempty"`
}

// UsageDimensions groups all quota dimensions.
type UsageDimensions struct {
	RunsToday           UsageDimension `json:"runs_today"`
	ConcurrentRuns      UsageDimension `json:"concurrent_runs"`
	ComputeCredit       UsageDimension `json:"compute_credit"`
	Projects            UsageDimension `json:"projects"`
	Members             UsageDimension `json:"members"`
	AIModelCalls        UsageDimension `json:"ai_model_calls_today"`
	AIAssistantMessages UsageDimension `json:"ai_assistant_messages_today"`
	RetentionDays       int            `json:"retention_days"`
	RegionsAvailable    int            `json:"regions_available"`
}

// UsageAlert represents a quota approaching/exceeded alert.
type UsageAlert struct {
	Type      string `json:"type"`
	Dimension string `json:"dimension"`
	Threshold int    `json:"threshold"`
	Message   string `json:"message"`
}

// UsageHistoryEntry is a single day's usage data.
type UsageHistoryEntry struct {
	Date             string `json:"date"`
	RunsCount        int64  `json:"runs_count"`
	ComputeCostMicro int64  `json:"compute_cost_microusd"`
	AITokens         int64  `json:"ai_tokens"`
	AICostMicro      int64  `json:"ai_cost_microusd"`
}

// UsageForecastResponse is the response for GET /v1/usage/forecast.
type UsageForecastResponse struct {
	ProjectedMonthlyRuns       int64   `json:"projected_monthly_runs"`
	ProjectedMonthlyComputeUsd float64 `json:"projected_monthly_compute_usd"`
	ProjectedMonthlyAICostUsd  float64 `json:"projected_monthly_ai_cost_usd"`
	RecommendedPlan            string  `json:"recommended_plan"`
	DaysUntilLimit             int     `json:"days_until_limit"`
	ProjectedOverageMicro      int64   `json:"projected_overage_microusd"`
	AddonSpendMicro            int64   `json:"addon_spend_microusd"`
	ScaleBreakeven             bool    `json:"scale_breakeven"`
}

// UsageService provides usage data aggregation.
type UsageService struct {
	store    Store
	enforcer *Enforcer
}

// NewUsageService creates a new usage service.
func NewUsageService(store Store, enforcer *Enforcer) *UsageService {
	return &UsageService{store: store, enforcer: enforcer}
}

// GetCurrentUsage returns the current billing period usage for an org.
func (s *UsageService) GetCurrentUsage(ctx context.Context, orgID string) (*CurrentUsageResponse, error) {
	limits, err := s.enforcer.GetOrgPlanLimits(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("getting org plan limits: %w", err)
	}

	// Get daily run count from Redis
	runsToday, err := s.enforcer.GetDailyRunCount(ctx, orgID)
	if err != nil {
		runsToday = 0
	}

	projectCount, err := s.store.CountProjectsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("counting org projects: %w", err)
	}

	memberCount, err := s.store.CountMembersByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("counting org members: %w", err)
	}

	concurrentRuns, err := s.store.CountExecutingRunsByOrg(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("counting org concurrent runs: %w", err)
	}

	now := time.Now().UTC()
	dayStart := now.UTC().Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)
	aiModelCallsToday, err := s.store.CountAIModelCallsByOrg(ctx, orgID, dayStart, dayEnd)
	if err != nil {
		return nil, fmt.Errorf("counting org ai model calls: %w", err)
	}

	// Get subscription for period info.
	sub, err := s.store.GetOrgSubscription(ctx, orgID)
	if err != nil && !errors.Is(err, ErrSubscriptionNotFound) {
		return nil, fmt.Errorf("getting org subscription: %w", err)
	}

	periodStart, periodEnd := usagePeriodWindow(now, limits.PlanTier, sub)

	periodSpend, err := s.store.SumOrgPeriodSpend(ctx, orgID, periodStart)
	if err != nil {
		return nil, fmt.Errorf("summing org period spend: %w", err)
	}

	computeUsed := periodSpend
	computeLimit := limits.ComputeCreditMicrousd

	// Region count
	regionCount := len(limits.AllowedRegions)
	if regionCount == 0 {
		regionCount = TotalRegions
	}

	aiModelCalls := UsageDimension{
		Used:    aiModelCallsToday,
		Limit:   int64(limits.MaxAIModelCallsPerDay),
		Percent: safePercent(aiModelCallsToday, int64(limits.MaxAIModelCallsPerDay)),
	}

	resp := &CurrentUsageResponse{
		OrgID: orgID,
		Plan:  string(limits.PlanTier),
		Period: PeriodInfo{
			Start: periodStart.Format("2006-01-02"),
			End:   periodEnd.Format("2006-01-02"),
		},
		Usage: UsageDimensions{
			RunsToday: UsageDimension{
				Used:    runsToday,
				Limit:   limits.MaxRunsPerDay,
				Percent: safePercent(runsToday, limits.MaxRunsPerDay),
			},
			ConcurrentRuns: UsageDimension{
				Used:    int64(concurrentRuns),
				Limit:   int64(limits.MaxConcurrentRuns),
				Percent: safePercent(int64(concurrentRuns), int64(limits.MaxConcurrentRuns)),
			},
			ComputeCredit: UsageDimension{
				Used:    computeUsed,
				Limit:   computeLimit,
				Percent: safePercent(computeUsed, computeLimit),
				Display: fmt.Sprintf("$%.2f / $%.2f", float64(computeUsed)/1000000, float64(computeLimit)/1000000),
			},
			Projects: UsageDimension{
				Used:    int64(projectCount),
				Limit:   int64(limits.MaxProjectsPerOrg),
				Percent: safePercent(int64(projectCount), int64(limits.MaxProjectsPerOrg)),
			},
			Members: UsageDimension{
				Used:    int64(memberCount),
				Limit:   int64(limits.MaxMembersPerOrg),
				Percent: safePercent(int64(memberCount), int64(limits.MaxMembersPerOrg)),
			},
			AIModelCalls:        aiModelCalls,
			AIAssistantMessages: aiModelCalls,
			RetentionDays:       limits.RetentionDays,
			RegionsAvailable:    regionCount,
		},
	}

	// Compute overage breakdown.
	resp.IncludedCreditMicro = computeLimit
	resp.PeriodSpendMicro = periodSpend
	resp.OverageMicro = computeOverageSpend(periodSpend, computeLimit)
	if computeLimit > 0 {
		resp.CreditUsedPercent = float64(periodSpend) / float64(computeLimit) * 100
	}
	if periodSpend < computeLimit {
		resp.CreditRemainingMicro = computeLimit - periodSpend
	}

	// Load active add-ons.
	if orgID != "" {
		addons, addonErr := s.store.ListActiveAddons(ctx, orgID)
		if addonErr == nil {
			for _, a := range addons {
				resp.ActiveAddons = append(resp.ActiveAddons, AddonSummary{
					Type:     string(a.AddonType),
					Quantity: a.Quantity,
				})
			}
		}
	}

	// Add alerts for dimensions approaching limits
	resp.Alerts = s.buildAlerts(resp.Usage)

	// Add overage alert for paid plans.
	if resp.OverageMicro > 0 && limits.PlanTier != domain.PlanFree {
		resp.Alerts = append(resp.Alerts, UsageAlert{
			Type:      "warning",
			Dimension: "overage",
			Message:   fmt.Sprintf("You're in overage ($%.2f over included credit). Set a spending limit to control costs.", float64(resp.OverageMicro)/1000000),
		})
	}

	// Surface payment status to the frontend for banners.
	if sub != nil && sub.PaymentStatus != "" && sub.PaymentStatus != "ok" {
		resp.PaymentStatus = sub.PaymentStatus
		if sub.GracePeriodEnd != nil {
			formatted := sub.GracePeriodEnd.Format(time.RFC3339)
			resp.GracePeriodEnd = &formatted
		}
	}

	return resp, nil
}

// GetUsageHistory returns historical usage for an org.
func (s *UsageService) GetUsageHistory(ctx context.Context, orgID string, from, to time.Time) ([]UsageHistoryEntry, error) {
	records, err := s.store.GetOrgUsageForPeriod(ctx, orgID, from, to)
	if err != nil {
		return nil, fmt.Errorf("getting usage history: %w", err)
	}

	dayMap := make(map[string]*UsageHistoryEntry)
	for _, r := range records {
		dateStr := r.PeriodDate.Format("2006-01-02")
		if entry, ok := dayMap[dateStr]; ok {
			entry.RunsCount += r.RunsCount
			entry.ComputeCostMicro += r.ComputeCostMicro
			entry.AITokens += r.AITokensTotal
			entry.AICostMicro += r.AICostMicro
		} else {
			dayMap[dateStr] = &UsageHistoryEntry{
				Date:             dateStr,
				RunsCount:        r.RunsCount,
				ComputeCostMicro: r.ComputeCostMicro,
				AITokens:         r.AITokensTotal,
				AICostMicro:      r.AICostMicro,
			}
		}
	}

	entries := make([]UsageHistoryEntry, 0, len(dayMap))
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		if entry, ok := dayMap[dateStr]; ok {
			entries = append(entries, *entry)
		} else {
			entries = append(entries, UsageHistoryEntry{Date: dateStr})
		}
	}

	return entries, nil
}

// GetUsageForecast projects usage for the current period.
func (s *UsageService) GetUsageForecast(ctx context.Context, orgID string) (*UsageForecastResponse, error) {
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)
	records, err := s.store.GetOrgUsageForPeriod(ctx, orgID, weekAgo, now)
	if err != nil {
		return nil, fmt.Errorf("getting usage for forecast: %w", err)
	}

	var totalRuns int64
	var totalCompute int64
	var totalAI int64
	days := 0
	daysSeen := make(map[string]bool)
	for _, r := range records {
		dateStr := r.PeriodDate.Format("2006-01-02")
		if !daysSeen[dateStr] {
			daysSeen[dateStr] = true
			days++
		}
		totalRuns += r.RunsCount
		totalCompute += r.ComputeCostMicro
		totalAI += r.AICostMicro
	}

	if days == 0 {
		days = 1
	}

	avgDailyRuns := totalRuns / int64(days)
	avgDailyCompute := totalCompute / int64(days)
	avgDailyAI := totalAI / int64(days)

	daysInMonth := 30
	projectedRuns := avgDailyRuns * int64(daysInMonth)
	projectedCompute := float64(avgDailyCompute*int64(daysInMonth)) / 1000000
	projectedAI := float64(avgDailyAI*int64(daysInMonth)) / 1000000

	limits, _ := s.enforcer.GetOrgPlanLimits(ctx, orgID)
	recommended := recommendPlan(projectedRuns, avgDailyCompute*int64(daysInMonth))

	daysUntilLimit := 0
	if limits.MaxRunsPerDay > 0 && avgDailyRuns > 0 {
		remaining := limits.MaxRunsPerDay - avgDailyRuns
		if remaining > 0 {
			daysUntilLimit = min(int(float64(limits.ComputeCreditMicrousd)/float64(avgDailyCompute)), 30)
		}
	}

	projectedComputeMicro := avgDailyCompute * int64(daysInMonth)
	projectedOverage := computeOverageSpend(projectedComputeMicro, limits.ComputeCreditMicrousd)

	// Sum active addon monthly costs (cents -> micro-USD: multiply by 10000).
	var addonSpendMicro int64
	addons, addonErr := s.store.ListActiveAddons(ctx, orgID)
	if addonErr == nil {
		for _, a := range addons {
			if pack, ok := AddonPacks[a.AddonType]; ok && a.Active && a.Quantity > 0 {
				addonSpendMicro += int64(pack.PriceCents) * int64(a.Quantity) * 10000
			}
		}
	}

	// Scale breakeven: true when a Pro user's total monthly spend
	// (subscription + addons + projected overage) >= Scale price ($99).
	totalProSpend := int64(PriceProMonthlyCents)*10000 + addonSpendMicro + projectedOverage
	scaleBreakeven := limits.PlanTier == domain.PlanPro && totalProSpend >= CreditScaleMicrousd

	return &UsageForecastResponse{
		ProjectedMonthlyRuns:       projectedRuns,
		ProjectedMonthlyComputeUsd: projectedCompute,
		ProjectedMonthlyAICostUsd:  projectedAI,
		RecommendedPlan:            recommended,
		DaysUntilLimit:             daysUntilLimit,
		ProjectedOverageMicro:      projectedOverage,
		AddonSpendMicro:            addonSpendMicro,
		ScaleBreakeven:             scaleBreakeven,
	}, nil
}

// GetProjectCosts delegates to the project_costs module.
func (s *UsageService) GetProjectCosts(ctx context.Context, orgID string, from, to time.Time) ([]ProjectCostEntry, error) {
	return GetProjectCosts(ctx, s.store, orgID, from, to)
}

// ExportUsageCSV delegates to the export module.
func (s *UsageService) ExportUsageCSV(ctx context.Context, orgID string, from, to time.Time) ([]byte, error) {
	return ExportCSV(ctx, s.store, orgID, ExportPeriod{From: from, To: to})
}

// ExportUsagePDF delegates to the PDF export module.
func (s *UsageService) ExportUsagePDF(ctx context.Context, orgID string, from, to time.Time) ([]byte, error) {
	return ExportPDF(ctx, s.store, orgID, ExportPeriod{From: from, To: to})
}

// GetSpendingLimit returns the current spending limit and overage info for an org.
func (s *UsageService) GetSpendingLimit(ctx context.Context, orgID string) (*SpendingLimitResponse, error) {
	sub, err := s.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if !errors.Is(err, ErrSubscriptionNotFound) {
			return nil, fmt.Errorf("getting org subscription: %w", err)
		}

		limits := GetPlanLimits(domain.PlanFree)
		periodStart, _ := usagePeriodWindow(time.Now().UTC(), domain.PlanFree, nil)
		periodSpend, spendErr := s.store.SumOrgPeriodSpend(ctx, orgID, periodStart)
		if spendErr != nil {
			return nil, fmt.Errorf("summing free-tier spend: %w", spendErr)
		}
		overageSpend := computeOverageSpend(periodSpend, limits.ComputeCreditMicrousd)

		return &SpendingLimitResponse{
			OrgID:             orgID,
			PlanTier:          string(domain.PlanFree),
			SpendingLimitUsd:  0,
			LimitAction:       "reject",
			CurrentSpendUsd:   float64(periodSpend) / 1000000,
			IncludedCreditUsd: float64(limits.ComputeCreditMicrousd) / 1000000,
			OverageSpendUsd:   float64(overageSpend) / 1000000,
			IsHardCapped:      true,
		}, nil
	}

	limits := GetPlanLimits(domain.PlanTier(sub.PlanTier))
	periodStart, _ := usagePeriodWindow(time.Now().UTC(), limits.PlanTier, sub)
	periodSpend, _ := s.store.SumOrgPeriodSpend(ctx, orgID, periodStart)
	overageSpend := computeOverageSpend(periodSpend, limits.ComputeCreditMicrousd)

	if limits.PlanTier == domain.PlanFree {
		return &SpendingLimitResponse{
			OrgID:             orgID,
			PlanTier:          string(domain.PlanFree),
			SpendingLimitUsd:  0,
			LimitAction:       "reject",
			CurrentSpendUsd:   float64(periodSpend) / 1000000,
			IncludedCreditUsd: float64(limits.ComputeCreditMicrousd) / 1000000,
			OverageSpendUsd:   float64(overageSpend) / 1000000,
			IsHardCapped:      true,
		}, nil
	}

	return &SpendingLimitResponse{
		OrgID:             orgID,
		PlanTier:          sub.PlanTier,
		SpendingLimitUsd:  float64(sub.SpendingLimitMicrousd) / 1000000,
		LimitAction:       sub.LimitAction,
		CurrentSpendUsd:   float64(periodSpend) / 1000000,
		IncludedCreditUsd: float64(limits.ComputeCreditMicrousd) / 1000000,
		OverageSpendUsd:   float64(overageSpend) / 1000000,
		IsHardCapped:      sub.SpendingLimitMicrousd == 0,
	}, nil
}

// SetSpendingLimit validates and updates the spending limit for an org.
func (s *UsageService) SetSpendingLimit(ctx context.Context, orgID string, limitMicrousd int64, action string) error {
	sub, err := s.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return fmt.Errorf("spending limits are not available on the Free plan")
		}
		return fmt.Errorf("getting org subscription: %w", err)
	}

	tier := domain.PlanTier(sub.PlanTier)
	if tier == domain.PlanFree {
		return fmt.Errorf("spending limits are not available on the Free plan")
	}

	if limitMicrousd < 0 {
		return fmt.Errorf("spending limit must be non-negative")
	}

	maxLimit := MaxSpendingLimit(tier)
	if maxLimit >= 0 && limitMicrousd > maxLimit {
		return fmt.Errorf("spending limit exceeds maximum of $%.2f for %s plan", float64(maxLimit)/1000000, tier)
	}

	if action != "reject" && action != "notify" {
		return fmt.Errorf("limit_action must be 'reject' or 'notify'")
	}

	return s.store.UpdateSpendingLimit(ctx, orgID, limitMicrousd, action)
}

// PreviewDowngrade delegates to the downgrade module.
func (s *UsageService) PreviewDowngrade(ctx context.Context, orgID string, targetTier domain.PlanTier) (*DowngradeImpact, error) {
	return PreviewDowngrade(ctx, s.store, orgID, targetTier)
}

// DetectAnomalies runs anomaly detection for a single org, using org-specific thresholds if configured.
func (s *UsageService) DetectAnomalies(ctx context.Context, orgID string) ([]AnomalyAlert, error) {
	cfg := DefaultAnomalyConfig()
	sub, err := s.store.GetOrgSubscription(ctx, orgID)
	if err == nil && sub != nil {
		if sub.AnomalyThresholdWarning > 0 {
			cfg.WarningThreshold = sub.AnomalyThresholdWarning
		}
		if sub.AnomalyThresholdCritical > 0 {
			cfg.CriticalThreshold = sub.AnomalyThresholdCritical
		}
	}
	detector := NewAnomalyDetectorWithConfig(s.store, cfg)
	return detector.DetectAnomalies(ctx, []string{orgID})
}

// ProjectBudgetResponse is the API response for project budget queries.
type ProjectBudgetResponse struct {
	ProjectID          string  `json:"project_id"`
	MonthlyBudgetMicro int64   `json:"monthly_budget_microusd"`
	BudgetAction       string  `json:"budget_action"`
	CurrentSpendMicro  int64   `json:"current_spend_microusd"`
	PercentUsed        float64 `json:"percent_used"`
}

// GetProjectBudget returns the budget and current spend for a project.
func (s *UsageService) GetProjectBudget(ctx context.Context, projectID string) (*ProjectBudgetResponse, error) {
	budget, action, err := s.store.GetProjectBudget(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("getting project budget: %w", err)
	}

	periodStart := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	spend, err := s.store.GetProjectPeriodSpend(ctx, projectID, periodStart)
	if err != nil {
		return nil, fmt.Errorf("getting project period spend: %w", err)
	}

	var pct float64
	if budget > 0 {
		pct = float64(spend) / float64(budget) * 100
	}

	return &ProjectBudgetResponse{
		ProjectID:          projectID,
		MonthlyBudgetMicro: budget,
		BudgetAction:       action,
		CurrentSpendMicro:  spend,
		PercentUsed:        pct,
	}, nil
}

// SetProjectBudget validates and stores a project budget.
func (s *UsageService) SetProjectBudget(ctx context.Context, projectID string, budgetMicro int64, action string) error {
	if action != "reject" && action != "notify" {
		return fmt.Errorf("budget_action must be 'reject' or 'notify'")
	}
	if budgetMicro < 0 {
		budgetMicro = -1 // normalize to "no budget"
	}
	return s.store.SetProjectBudget(ctx, projectID, budgetMicro, action)
}

// AnomalyConfigResponse is the API response for anomaly threshold queries.
type AnomalyConfigResponse struct {
	WarningThreshold  float64 `json:"warning_threshold"`
	CriticalThreshold float64 `json:"critical_threshold"`
}

// GetAnomalyConfig returns the current anomaly detection thresholds for an org.
func (s *UsageService) GetAnomalyConfig(ctx context.Context, orgID string) (*AnomalyConfigResponse, error) {
	sub, err := s.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			defaults := DefaultAnomalyConfig()
			return &AnomalyConfigResponse{
				WarningThreshold:  defaults.WarningThreshold,
				CriticalThreshold: defaults.CriticalThreshold,
			}, nil
		}
		return nil, fmt.Errorf("getting org subscription: %w", err)
	}

	warning := sub.AnomalyThresholdWarning
	critical := sub.AnomalyThresholdCritical
	if warning <= 0 {
		warning = spikeWarning
	}
	if critical <= 0 {
		critical = spikeCritical
	}

	return &AnomalyConfigResponse{
		WarningThreshold:  warning,
		CriticalThreshold: critical,
	}, nil
}

// SetAnomalyConfig validates and updates the anomaly detection thresholds for an org.
func (s *UsageService) SetAnomalyConfig(ctx context.Context, orgID string, warning, critical float64) error {
	if warning <= 1.0 {
		return fmt.Errorf("warning_threshold must be greater than 1.0")
	}
	if critical <= warning {
		return fmt.Errorf("critical_threshold must be greater than warning_threshold")
	}
	return s.store.UpdateAnomalyThresholds(ctx, orgID, warning, critical)
}

func (s *UsageService) buildAlerts(usage UsageDimensions) []UsageAlert {
	var alerts []UsageAlert

	if usage.RunsToday.Percent >= 80 {
		alerts = append(alerts, UsageAlert{
			Type:      "approaching_limit",
			Dimension: "runs_today",
			Threshold: 80,
			Message:   fmt.Sprintf("You've used %.1f%% of daily runs", usage.RunsToday.Percent),
		})
	}
	if usage.ComputeCredit.Percent >= 80 {
		alerts = append(alerts, UsageAlert{
			Type:      "approaching_limit",
			Dimension: "compute_credit",
			Threshold: 80,
			Message:   fmt.Sprintf("You've used %.1f%% of compute credit", usage.ComputeCredit.Percent),
		})
	}
	if usage.Projects.Percent >= 80 {
		alerts = append(alerts, UsageAlert{
			Type:      "approaching_limit",
			Dimension: "projects",
			Threshold: 80,
			Message:   fmt.Sprintf("You've used %.1f%% of project slots", usage.Projects.Percent),
		})
	}

	return alerts
}

// EmailPreferencesResponse is the response for email preferences queries.
type EmailPreferencesResponse struct {
	MonthlyUsageEmail bool `json:"monthly_usage_email"`
}

// GetEmailPreferences returns the email preferences for an org.
func (s *UsageService) GetEmailPreferences(ctx context.Context, orgID string) (*EmailPreferencesResponse, error) {
	sub, err := s.store.GetOrgSubscription(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return &EmailPreferencesResponse{MonthlyUsageEmail: true}, nil
		}
		return nil, fmt.Errorf("getting email preferences: %w", err)
	}
	return &EmailPreferencesResponse{MonthlyUsageEmail: sub.MonthlyUsageEmail}, nil
}

// UpdateEmailPreferences updates the monthly usage email preference for an org.
func (s *UsageService) UpdateEmailPreferences(ctx context.Context, orgID string, monthlyUsageEmail bool) error {
	return s.store.UpdateMonthlyUsageEmail(ctx, orgID, monthlyUsageEmail)
}

func safePercent(used, limit int64) float64 {
	if limit <= 0 {
		return 0
	}
	return float64(used) / float64(limit) * 100
}

func recommendPlan(_ int64, monthlyComputeMicro int64) string {
	if monthlyComputeMicro <= CreditFreeMicrousd {
		return string(domain.PlanFree)
	}
	if monthlyComputeMicro <= CreditStarterMicrousd {
		return string(domain.PlanStarter)
	}
	if monthlyComputeMicro <= CreditProMicrousd {
		return string(domain.PlanPro)
	}
	if monthlyComputeMicro <= CreditScaleMicrousd {
		return string(domain.PlanScale)
	}
	return string(domain.PlanEnterprise)
}
