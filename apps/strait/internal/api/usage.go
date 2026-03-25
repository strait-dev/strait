package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"strait/internal/billing"
	"strait/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) validateCallerOrgAccess(ctx context.Context, orgID string) error {
	if scopesFromContext(ctx) == nil {
		return nil
	}
	projectID := projectIDFromContext(ctx)
	if projectID == "" || s.billingEnforcer == nil {
		return fmt.Errorf("cannot determine caller organization from project context")
	}
	callerOrg, err := s.billingEnforcer.GetActiveProjectOrgID(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to resolve active project org: %w", err)
	}
	if callerOrg != orgID {
		return fmt.Errorf("org_id does not match the caller's organization")
	}
	return nil
}

func (s *Server) validateProjectBelongsToCallerOrg(ctx context.Context, targetProjectID string) error {
	callerProjectID := projectIDFromContext(ctx)
	if s.billingEnforcer == nil {
		if callerProjectID != "" {
			return fmt.Errorf("ownership validation unavailable: billing enforcer not configured")
		}
		return nil
	}
	if callerProjectID == "" {
		return fmt.Errorf("no project context")
	}
	callerOrg, err := s.billingEnforcer.GetActiveProjectOrgID(ctx, callerProjectID)
	if err != nil {
		return fmt.Errorf("failed to resolve caller org: %w", err)
	}
	if callerOrg == "" {
		return fmt.Errorf("caller project has no associated organization")
	}
	targetOrg, err := s.billingEnforcer.GetActiveProjectOrgID(ctx, targetProjectID)
	if err != nil {
		return fmt.Errorf("failed to resolve target project org: %w", err)
	}
	if targetOrg == "" {
		return fmt.Errorf("target project has no associated organization")
	}
	if callerOrg != targetOrg {
		return fmt.Errorf("project does not belong to caller's organization")
	}
	return nil
}

func (s *Server) resolveUsageOrgIDTyped(ctx context.Context, orgID string) (string, error) {
	if orgID == "" {
		return "", huma.Error400BadRequest("org_id query parameter is required")
	}
	if err := s.validateCallerOrgAccess(ctx, orgID); err != nil {
		if scopesFromContext(ctx) != nil {
			slog.Error("org access validation failed", "project_id", projectIDFromContext(ctx), "error", err)
		}
		return "", huma.Error403Forbidden(err.Error())
	}
	return orgID, nil
}

func parseDateRangeTyped(fromStr, toStr string) (time.Time, time.Time, error) {
	if fromStr == "" || toStr == "" {
		return time.Time{}, time.Time{}, huma.Error400BadRequest("from and to query parameters are required (format: YYYY-MM-DD)")
	}
	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, huma.Error400BadRequest("invalid from date format (expected YYYY-MM-DD)")
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		return time.Time{}, time.Time{}, huma.Error400BadRequest("invalid to date format (expected YYYY-MM-DD)")
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, huma.Error400BadRequest("to date must be after from date")
	}
	return from, to, nil
}

type GetCurrentUsageInput struct {
	OrgID string `query:"org_id"`
}
type GetCurrentUsageOutput struct{ Body any }

func (s *Server) handleGetCurrentUsage(ctx context.Context, input *GetCurrentUsageInput) (*GetCurrentUsageOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	usage, usageErr := s.usageService.GetCurrentUsage(ctx, orgID)
	if usageErr != nil {
		slog.Error("failed to get current usage", "error", usageErr)
		return nil, huma.Error500InternalServerError("failed to get usage data")
	}
	if scopesFromContext(ctx) != nil {
		usage.PaymentStatus = ""
		usage.GracePeriodEnd = nil
	}
	return &GetCurrentUsageOutput{Body: usage}, nil
}

type GetUsageHistoryInput struct {
	OrgID string `query:"org_id"`
	From  string `query:"from"`
	To    string `query:"to"`
}
type GetUsageHistoryOutput struct{ Body any }

func (s *Server) handleGetUsageHistory(ctx context.Context, input *GetUsageHistoryInput) (*GetUsageHistoryOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	from, to, err := parseDateRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	history, histErr := s.usageService.GetUsageHistory(ctx, orgID, from, to)
	if histErr != nil {
		slog.Error("failed to get usage history", "error", histErr)
		return nil, huma.Error500InternalServerError("failed to get usage history")
	}
	return &GetUsageHistoryOutput{Body: history}, nil
}

type GetUsageForecastInput struct {
	OrgID string `query:"org_id"`
}
type GetUsageForecastOutput struct{ Body any }

func (s *Server) handleGetUsageForecast(ctx context.Context, input *GetUsageForecastInput) (*GetUsageForecastOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	forecast, fErr := s.usageService.GetUsageForecast(ctx, orgID)
	if fErr != nil {
		slog.Error("failed to get usage forecast", "error", fErr)
		return nil, huma.Error500InternalServerError("failed to get usage forecast")
	}
	return &GetUsageForecastOutput{Body: forecast}, nil
}

type GetProjectCostsInput struct {
	OrgID string `query:"org_id"`
	From  string `query:"from"`
	To    string `query:"to"`
}
type GetProjectCostsOutput struct{ Body any }

func (s *Server) handleGetProjectCosts(ctx context.Context, input *GetProjectCostsInput) (*GetProjectCostsOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	from, to, err := parseDateRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	costs, cErr := s.usageService.GetProjectCosts(ctx, orgID, from, to)
	if cErr != nil {
		slog.Error("failed to get project costs", "error", cErr)
		return nil, huma.Error500InternalServerError("failed to get project costs")
	}
	return &GetProjectCostsOutput{Body: costs}, nil
}

type GetCostEstimateInput struct {
	Preset     string `query:"preset"`
	TimeoutStr string `query:"timeout_secs"`
	OrgID      string `query:"org_id"`
}
type GetCostEstimateOutput struct{ Body any }

func (s *Server) handleGetCostEstimate(ctx context.Context, input *GetCostEstimateInput) (*GetCostEstimateOutput, error) {
	if input.Preset == "" {
		return nil, huma.Error400BadRequest("preset query parameter is required")
	}
	if input.TimeoutStr == "" {
		return nil, huma.Error400BadRequest("timeout_secs query parameter is required")
	}
	timeoutSecs, err := strconv.Atoi(input.TimeoutStr)
	if err != nil || timeoutSecs <= 0 {
		return nil, huma.Error400BadRequest("timeout_secs must be a positive integer")
	}
	var creditRemaining int64
	if s.usageService != nil && input.OrgID != "" {
		if err := s.validateCallerOrgAccess(ctx, input.OrgID); err != nil {
			return nil, huma.Error403Forbidden(err.Error())
		}
		limit, limitErr := s.usageService.GetSpendingLimit(ctx, input.OrgID)
		if limitErr == nil {
			creditRemaining = int64((limit.IncludedCreditUsd - limit.CurrentSpendUsd) * 1000000)
			creditRemaining = max(creditRemaining, 0)
		}
	}
	estimate, err := billing.EstimateJobCost(input.Preset, timeoutSecs, creditRemaining)
	if err != nil {
		return nil, huma.Error400BadRequest(fmt.Sprintf("invalid preset: %v", err))
	}
	return &GetCostEstimateOutput{Body: estimate}, nil
}

type GetSpendingLimitInput struct {
	OrgID string `query:"org_id"`
}
type GetSpendingLimitOutput struct{ Body any }

func (s *Server) handleGetSpendingLimit(ctx context.Context, input *GetSpendingLimitInput) (*GetSpendingLimitOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	limit, lErr := s.usageService.GetSpendingLimit(ctx, orgID)
	if lErr != nil {
		slog.Error("failed to get spending limit", "error", lErr)
		return nil, huma.Error500InternalServerError("failed to get spending limit")
	}
	return &GetSpendingLimitOutput{Body: limit}, nil
}

type updateSpendingLimitRequest struct {
	LimitMicrousd int64  `json:"limit_microusd"`
	Action        string `json:"action"`
}
type UpdateSpendingLimitInput struct {
	OrgID string `query:"org_id"`
	Body  updateSpendingLimitRequest
}
type UpdateSpendingLimitOutput struct{ Body map[string]string }

func (s *Server) handleUpdateSpendingLimit(ctx context.Context, input *UpdateSpendingLimitInput) (*UpdateSpendingLimitOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	if err := s.usageService.SetSpendingLimit(ctx, orgID, input.Body.LimitMicrousd, input.Body.Action); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return &UpdateSpendingLimitOutput{Body: map[string]string{"status": "updated"}}, nil
}

// Email Preferences.

type GetEmailPreferencesInput struct {
	OrgID string `query:"org_id"`
}
type GetEmailPreferencesOutput struct{ Body any }

func (s *Server) handleGetEmailPreferences(ctx context.Context, input *GetEmailPreferencesInput) (*GetEmailPreferencesOutput, error) {
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	if s.usageService == nil {
		return &GetEmailPreferencesOutput{Body: &billing.EmailPreferencesResponse{MonthlyUsageEmail: true}}, nil
	}
	prefs, pErr := s.usageService.GetEmailPreferences(ctx, orgID)
	if pErr != nil {
		slog.Warn("failed to get email preferences", "org_id", orgID, "error", pErr)
		return nil, huma.Error500InternalServerError("failed to get email preferences")
	}
	return &GetEmailPreferencesOutput{Body: prefs}, nil
}

type updateEmailPreferencesRequest struct {
	MonthlyUsageEmail bool `json:"monthly_usage_email"`
}
type UpdateEmailPreferencesInput struct {
	OrgID string `query:"org_id"`
	Body  updateEmailPreferencesRequest
}
type UpdateEmailPreferencesOutput struct{ Body map[string]string }

func (s *Server) handleUpdateEmailPreferences(ctx context.Context, input *UpdateEmailPreferencesInput) (*UpdateEmailPreferencesOutput, error) {
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	if err := s.usageService.UpdateEmailPreferences(ctx, orgID, input.Body.MonthlyUsageEmail); err != nil {
		return nil, huma.Error500InternalServerError("failed to update email preferences")
	}
	return &UpdateEmailPreferencesOutput{Body: map[string]string{"status": "updated"}}, nil
}

type GetDowngradePreviewInput struct {
	OrgID      string `query:"org_id"`
	TargetTier string `query:"target_tier"`
}
type GetDowngradePreviewOutput struct{ Body any }

func (s *Server) handleGetDowngradePreview(ctx context.Context, input *GetDowngradePreviewInput) (*GetDowngradePreviewOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	if input.TargetTier == "" {
		return nil, huma.Error400BadRequest("target_tier query parameter is required")
	}
	tier := domain.PlanTier(input.TargetTier)
	if _, exists := billing.Plans[tier]; !exists {
		return nil, huma.Error400BadRequest("invalid target_tier")
	}
	impact, iErr := s.usageService.PreviewDowngrade(ctx, orgID, tier)
	if iErr != nil {
		slog.Error("failed to preview downgrade", "error", iErr)
		return nil, huma.Error500InternalServerError("failed to preview downgrade")
	}
	return &GetDowngradePreviewOutput{Body: impact}, nil
}

type ExportUsageInput struct {
	OrgID  string `query:"org_id"`
	From   string `query:"from"`
	To     string `query:"to"`
	Format string `query:"format"`
}

// ExportUsageOutput uses any Body because the handler writes raw CSV/PDF bytes
// directly to the response writer. A nil return signals the response was already written.
type ExportUsageOutput struct {
	Body any
}

func (s *Server) handleExportUsage(ctx context.Context, input *ExportUsageInput) (*ExportUsageOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	from, to, err := parseDateRangeTyped(input.From, input.To)
	if err != nil {
		return nil, err
	}
	format := input.Format
	if format == "" {
		format = "csv"
	}

	// Retrieve the raw response writer for binary output.
	w := responseWriterFromContext(ctx)
	if w == nil {
		return nil, huma.Error500InternalServerError("internal error")
	}

	switch format {
	case "csv":
		csvData, csvErr := s.usageService.ExportUsageCSV(ctx, orgID, from, to)
		if csvErr != nil {
			slog.Error("failed to export usage", "error", csvErr)
			return nil, huma.Error500InternalServerError("failed to export usage")
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=usage_%s.csv", orgID))
		w.WriteHeader(http.StatusOK)
		w.Write(csvData) //nolint:errcheck,gosec
	case "pdf":
		pdfData, pdfErr := s.usageService.ExportUsagePDF(ctx, orgID, from, to)
		if pdfErr != nil {
			slog.Error("failed to export usage PDF", "error", pdfErr)
			return nil, huma.Error500InternalServerError("failed to export usage")
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=usage_%s.pdf", orgID))
		w.WriteHeader(http.StatusOK)
		w.Write(pdfData) //nolint:errcheck,gosec
	default:
		return nil, huma.Error400BadRequest("unsupported format, use csv or pdf")
	}

	// Return nil to signal that the response was already written.
	return nil, nil
}

type GetAnomalyAlertsInput struct {
	OrgID string `query:"org_id"`
}
type GetAnomalyAlertsOutput struct{ Body []billing.AnomalyAlert }

func (s *Server) handleGetAnomalyAlerts(ctx context.Context, input *GetAnomalyAlertsInput) (*GetAnomalyAlertsOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	alerts, aErr := s.usageService.DetectAnomalies(ctx, orgID)
	if aErr != nil {
		slog.Error("failed to detect anomalies", "error", aErr)
		return nil, huma.Error500InternalServerError("failed to detect anomalies")
	}
	if alerts == nil {
		alerts = []billing.AnomalyAlert{}
	}
	return &GetAnomalyAlertsOutput{Body: alerts}, nil
}

type GetProjectBudgetInput struct {
	ProjectID string `query:"project_id"`
}
type GetProjectBudgetOutput struct{ Body any }

func (s *Server) handleGetProjectBudget(ctx context.Context, input *GetProjectBudgetInput) (*GetProjectBudgetOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	if input.ProjectID == "" {
		return nil, huma.Error400BadRequest("project_id query parameter is required")
	}
	if err := s.validateProjectBelongsToCallerOrg(ctx, input.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("access denied")
	}
	budget, bErr := s.usageService.GetProjectBudget(ctx, input.ProjectID)
	if bErr != nil {
		slog.Error("failed to get project budget", "error", bErr)
		return nil, huma.Error500InternalServerError("failed to get project budget")
	}
	return &GetProjectBudgetOutput{Body: budget}, nil
}

type updateProjectBudgetRequest struct {
	ProjectID   string `json:"project_id"`
	BudgetMicro int64  `json:"budget_microusd"`
	Action      string `json:"action"`
}
type UpdateProjectBudgetInput struct{ Body updateProjectBudgetRequest }
type UpdateProjectBudgetOutput struct{ Body map[string]string }

func (s *Server) handleUpdateProjectBudget(ctx context.Context, input *UpdateProjectBudgetInput) (*UpdateProjectBudgetOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	req := input.Body
	if req.ProjectID == "" {
		return nil, huma.Error400BadRequest("project_id is required")
	}
	if err := s.validateProjectBelongsToCallerOrg(ctx, req.ProjectID); err != nil {
		return nil, huma.Error403Forbidden("access denied")
	}
	if err := s.usageService.SetProjectBudget(ctx, req.ProjectID, req.BudgetMicro, req.Action); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return &UpdateProjectBudgetOutput{Body: map[string]string{"status": "updated"}}, nil
}

type GetAnomalyConfigInput struct {
	OrgID string `query:"org_id"`
}
type GetAnomalyConfigOutput struct{ Body any }

func (s *Server) handleGetAnomalyConfig(ctx context.Context, input *GetAnomalyConfigInput) (*GetAnomalyConfigOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	cfg, cErr := s.usageService.GetAnomalyConfig(ctx, orgID)
	if cErr != nil {
		slog.Error("failed to get anomaly config", "error", cErr)
		return nil, huma.Error500InternalServerError("failed to get anomaly config")
	}
	return &GetAnomalyConfigOutput{Body: cfg}, nil
}

type updateAnomalyConfigRequest struct {
	Warning  float64 `json:"warning_threshold"`
	Critical float64 `json:"critical_threshold"`
}
type UpdateAnomalyConfigInput struct {
	OrgID string `query:"org_id"`
	Body  updateAnomalyConfigRequest
}
type UpdateAnomalyConfigOutput struct{ Body map[string]string }

func (s *Server) handleUpdateAnomalyConfig(ctx context.Context, input *UpdateAnomalyConfigInput) (*UpdateAnomalyConfigOutput, error) {
	if s.usageService == nil {
		return nil, huma.Error501NotImplemented("usage service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	if err := s.usageService.SetAnomalyConfig(ctx, orgID, input.Body.Warning, input.Body.Critical); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return &UpdateAnomalyConfigOutput{Body: map[string]string{"status": "updated"}}, nil
}

// Referral handlers.
type createReferralCodeRequest struct {
	OrgID string `json:"org_id"`
}
type CreateReferralCodeInput struct{ Body createReferralCodeRequest }
type CreateReferralCodeOutput struct{ Body any }

func (s *Server) handleCreateReferralCode(ctx context.Context, input *CreateReferralCodeInput) (*CreateReferralCodeOutput, error) {
	if s.referralService == nil {
		return nil, huma.Error501NotImplemented("referral service not configured")
	}
	if input.Body.OrgID == "" {
		return nil, huma.Error400BadRequest("org_id is required")
	}
	if err := s.validateCallerOrgAccess(ctx, input.Body.OrgID); err != nil {
		return nil, huma.Error403Forbidden(err.Error())
	}
	referral, err := s.referralService.GenerateCode(ctx, input.Body.OrgID)
	if err != nil {
		slog.Error("failed to create referral code", "error", err)
		return nil, huma.Error500InternalServerError("failed to create referral code")
	}
	return &CreateReferralCodeOutput{Body: referral}, nil
}

type activateReferralRequest struct {
	Code          string `json:"code"`
	ReferredOrgID string `json:"referred_org_id"`
	ReferredEmail string `json:"referred_email"`
}
type ActivateReferralInput struct{ Body activateReferralRequest }
type ActivateReferralOutput struct{ Body any }

func (s *Server) handleActivateReferral(ctx context.Context, input *ActivateReferralInput) (*ActivateReferralOutput, error) {
	if s.referralService == nil {
		return nil, huma.Error501NotImplemented("referral service not configured")
	}
	req := input.Body
	if req.Code == "" || req.ReferredOrgID == "" {
		return nil, huma.Error400BadRequest("code and referred_org_id are required")
	}
	if err := s.validateCallerOrgAccess(ctx, req.ReferredOrgID); err != nil {
		return nil, huma.Error403Forbidden(err.Error())
	}
	referral, err := s.referralService.ActivateReferral(ctx, req.Code, req.ReferredOrgID, req.ReferredEmail)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return &ActivateReferralOutput{Body: referral}, nil
}

type ListReferralsInput struct {
	OrgID string `query:"org_id"`
}
type ListReferralsOutput struct{ Body []billing.Referral }

func (s *Server) handleListReferrals(ctx context.Context, input *ListReferralsInput) (*ListReferralsOutput, error) {
	if s.referralService == nil {
		return nil, huma.Error501NotImplemented("referral service not configured")
	}
	orgID, err := s.resolveUsageOrgIDTyped(ctx, input.OrgID)
	if err != nil {
		return nil, err
	}
	referrals, rErr := s.referralService.ListReferrals(ctx, orgID)
	if rErr != nil {
		slog.Error("failed to list referrals", "error", rErr)
		return nil, huma.Error500InternalServerError("failed to list referrals")
	}
	if referrals == nil {
		referrals = []billing.Referral{}
	}
	return &ListReferralsOutput{Body: referrals}, nil
}

type CheckOrgLimitInput struct {
	UserID   string `query:"user_id"`
	PlanTier string `query:"plan_tier"`
}
type CheckOrgLimitOutput struct{ Body map[string]string }

func (s *Server) handleCheckOrgLimit(ctx context.Context, input *CheckOrgLimitInput) (*CheckOrgLimitOutput, error) {
	if scopesFromContext(ctx) != nil {
		return nil, huma.Error403Forbidden("org limit check requires internal secret")
	}
	if s.billingEnforcer == nil {
		return &CheckOrgLimitOutput{Body: map[string]string{"status": "allowed"}}, nil
	}
	if input.UserID == "" {
		return nil, huma.Error400BadRequest("user_id query parameter is required")
	}
	planTier := domain.PlanTier(input.PlanTier)
	if planTier == "" {
		planTier = domain.PlanFree
	}
	if err := s.billingEnforcer.CheckOrgCreationLimit(ctx, input.UserID, planTier); err != nil {
		var le *billing.LimitError
		if errors.As(err, &le) {
			return nil, le
		}
		slog.Error("failed to check org creation limit", "error", err)
		return nil, huma.Error500InternalServerError("failed to check org creation limit")
	}
	return &CheckOrgLimitOutput{Body: map[string]string{"status": "allowed"}}, nil
}
