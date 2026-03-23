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
)

// validateCallerOrgAccess checks that a non-internal project-scoped caller
// belongs to the given org. Returns nil for internal-secret callers (no scopes
// in context).
func (s *Server) validateCallerOrgAccess(ctx context.Context, orgID string) error {
	if scopesFromContext(ctx) == nil {
		return nil // internal-secret caller, trusted
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

// validateProjectBelongsToCallerOrg checks that the target project belongs to
// the same org as the caller's project context. Unlike validateCallerOrgAccess,
// this runs for ALL callers including internal-secret, because project-scoped
// endpoints must always verify ownership.
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

// resolveUsageOrgID extracts org_id from the request query, enforcing tenant
// isolation for non-internal project-scoped callers. Returns the org_id or
// writes an error response.
func (s *Server) resolveUsageOrgID(w http.ResponseWriter, r *http.Request) (string, bool) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		respondError(w, r, http.StatusBadRequest, "org_id query parameter is required")
		return "", false
	}

	if err := s.validateCallerOrgAccess(r.Context(), orgID); err != nil {
		if scopesFromContext(r.Context()) != nil {
			projectID := projectIDFromContext(r.Context())
			slog.Error("org access validation failed", "project_id", projectID, "error", err)
		}
		respondError(w, r, http.StatusForbidden, err.Error())
		return "", false
	}

	return orgID, true
}

func (s *Server) handleGetCurrentUsage(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	usage, err := s.usageService.GetCurrentUsage(r.Context(), orgID)
	if err != nil {
		slog.Error("failed to get current usage", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to get usage data")
		return
	}

	// Strip internal payment fields for non-internal callers (API keys, OIDC).
	// Only internal-secret callers (frontend app) should see payment status.
	if scopesFromContext(r.Context()) != nil {
		usage.PaymentStatus = ""
		usage.GracePeriodEnd = nil
	}

	respondJSON(w, http.StatusOK, usage)
}

func (s *Server) handleGetUsageHistory(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	history, err := s.usageService.GetUsageHistory(r.Context(), orgID, from, to)
	if err != nil {
		slog.Error("failed to get usage history", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to get usage history")
		return
	}

	respondJSON(w, http.StatusOK, history)
}

func (s *Server) handleGetUsageForecast(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	forecast, err := s.usageService.GetUsageForecast(r.Context(), orgID)
	if err != nil {
		slog.Error("failed to get usage forecast", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to get usage forecast")
		return
	}

	respondJSON(w, http.StatusOK, forecast)
}

func (s *Server) handleGetProjectCosts(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	costs, err := s.usageService.GetProjectCosts(r.Context(), orgID, from, to)
	if err != nil {
		slog.Error("failed to get project costs", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to get project costs")
		return
	}

	respondJSON(w, http.StatusOK, costs)
}

func (s *Server) handleGetCostEstimate(w http.ResponseWriter, r *http.Request) {
	preset := r.URL.Query().Get("preset")
	if preset == "" {
		respondError(w, r, http.StatusBadRequest, "preset query parameter is required")
		return
	}

	timeoutStr := r.URL.Query().Get("timeout_secs")
	if timeoutStr == "" {
		respondError(w, r, http.StatusBadRequest, "timeout_secs query parameter is required")
		return
	}
	timeoutSecs, err := strconv.Atoi(timeoutStr)
	if err != nil || timeoutSecs <= 0 {
		respondError(w, r, http.StatusBadRequest, "timeout_secs must be a positive integer")
		return
	}

	// Use 0 credit remaining by default; if usage service is available, compute it.
	var creditRemaining int64
	if s.usageService != nil {
		orgID := r.URL.Query().Get("org_id")
		if orgID != "" {
			if err := s.validateCallerOrgAccess(r.Context(), orgID); err != nil {
				respondError(w, r, http.StatusForbidden, err.Error())
				return
			}
			limit, limitErr := s.usageService.GetSpendingLimit(r.Context(), orgID)
			if limitErr == nil {
				creditRemaining = int64((limit.IncludedCreditUsd - limit.CurrentSpendUsd) * 1000000)
				creditRemaining = max(creditRemaining, 0)
			}
		}
	}

	estimate, err := billing.EstimateJobCost(preset, timeoutSecs, creditRemaining)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, fmt.Sprintf("invalid preset: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, estimate)
}

func (s *Server) handleGetSpendingLimit(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	limit, err := s.usageService.GetSpendingLimit(r.Context(), orgID)
	if err != nil {
		slog.Error("failed to get spending limit", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to get spending limit")
		return
	}

	respondJSON(w, http.StatusOK, limit)
}

func (s *Server) handleUpdateSpendingLimit(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	var req struct {
		LimitMicrousd int64  `json:"limit_microusd"`
		Action        string `json:"action"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.usageService.SetSpendingLimit(r.Context(), orgID, req.LimitMicrousd, req.Action); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleGetDowngradePreview(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	targetTier := r.URL.Query().Get("target_tier")
	if targetTier == "" {
		respondError(w, r, http.StatusBadRequest, "target_tier query parameter is required")
		return
	}

	tier := domain.PlanTier(targetTier)
	if _, exists := billing.Plans[tier]; !exists {
		respondError(w, r, http.StatusBadRequest, "invalid target_tier")
		return
	}

	impact, err := s.usageService.PreviewDowngrade(r.Context(), orgID, tier)
	if err != nil {
		slog.Error("failed to preview downgrade", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to preview downgrade")
		return
	}

	respondJSON(w, http.StatusOK, impact)
}

func (s *Server) handleExportUsage(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "csv"
	}

	switch format {
	case "csv":
		csvData, err := s.usageService.ExportUsageCSV(r.Context(), orgID, from, to)
		if err != nil {
			slog.Error("failed to export usage", "error", err)
			respondError(w, r, http.StatusInternalServerError, "failed to export usage")
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=usage_%s.csv", orgID))
		w.WriteHeader(http.StatusOK)
		w.Write(csvData) //nolint:errcheck,gosec // best-effort response write

	case "pdf":
		pdfData, err := s.usageService.ExportUsagePDF(r.Context(), orgID, from, to)
		if err != nil {
			slog.Error("failed to export usage PDF", "error", err)
			respondError(w, r, http.StatusInternalServerError, "failed to export usage")
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=usage_%s.pdf", orgID))
		w.WriteHeader(http.StatusOK)
		w.Write(pdfData) //nolint:errcheck,gosec // best-effort response write

	default:
		respondError(w, r, http.StatusBadRequest, "unsupported format, use csv or pdf")
	}
}

func (s *Server) handleGetAnomalyAlerts(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	alerts, err := s.usageService.DetectAnomalies(r.Context(), orgID)
	if err != nil {
		slog.Error("failed to detect anomalies", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to detect anomalies")
		return
	}

	if alerts == nil {
		alerts = []billing.AnomalyAlert{}
	}

	respondJSON(w, http.StatusOK, alerts)
}

func (s *Server) handleGetProjectBudget(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	projectID := r.URL.Query().Get("project_id")
	if projectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id query parameter is required")
		return
	}

	if err := s.validateProjectBelongsToCallerOrg(r.Context(), projectID); err != nil {
		respondError(w, r, http.StatusForbidden, "access denied")
		return
	}

	budget, err := s.usageService.GetProjectBudget(r.Context(), projectID)
	if err != nil {
		slog.Error("failed to get project budget", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to get project budget")
		return
	}

	respondJSON(w, http.StatusOK, budget)
}

func (s *Server) handleUpdateProjectBudget(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	var req struct {
		ProjectID   string `json:"project_id"`
		BudgetMicro int64  `json:"budget_microusd"`
		Action      string `json:"action"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProjectID == "" {
		respondError(w, r, http.StatusBadRequest, "project_id is required")
		return
	}

	if err := s.validateProjectBelongsToCallerOrg(r.Context(), req.ProjectID); err != nil {
		respondError(w, r, http.StatusForbidden, "access denied")
		return
	}

	if err := s.usageService.SetProjectBudget(r.Context(), req.ProjectID, req.BudgetMicro, req.Action); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (s *Server) handleGetAnomalyConfig(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	cfg, err := s.usageService.GetAnomalyConfig(r.Context(), orgID)
	if err != nil {
		slog.Error("failed to get anomaly config", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to get anomaly config")
		return
	}

	respondJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleUpdateAnomalyConfig(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	var req struct {
		Warning  float64 `json:"warning_threshold"`
		Critical float64 `json:"critical_threshold"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := s.usageService.SetAnomalyConfig(r.Context(), orgID, req.Warning, req.Critical); err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// Referral handlers.

func (s *Server) handleCreateReferralCode(w http.ResponseWriter, r *http.Request) {
	if s.referralService == nil {
		respondError(w, r, http.StatusNotImplemented, "referral service not configured")
		return
	}

	var req struct {
		OrgID string `json:"org_id"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OrgID == "" {
		respondError(w, r, http.StatusBadRequest, "org_id is required")
		return
	}

	if err := s.validateCallerOrgAccess(r.Context(), req.OrgID); err != nil {
		respondError(w, r, http.StatusForbidden, err.Error())
		return
	}

	referral, err := s.referralService.GenerateCode(r.Context(), req.OrgID)
	if err != nil {
		slog.Error("failed to create referral code", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to create referral code")
		return
	}

	respondJSON(w, http.StatusCreated, referral)
}

func (s *Server) handleActivateReferral(w http.ResponseWriter, r *http.Request) {
	if s.referralService == nil {
		respondError(w, r, http.StatusNotImplemented, "referral service not configured")
		return
	}

	var req struct {
		Code          string `json:"code"`
		ReferredOrgID string `json:"referred_org_id"`
		ReferredEmail string `json:"referred_email"`
	}
	if err := s.decodeJSON(r, &req); err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Code == "" || req.ReferredOrgID == "" {
		respondError(w, r, http.StatusBadRequest, "code and referred_org_id are required")
		return
	}

	if err := s.validateCallerOrgAccess(r.Context(), req.ReferredOrgID); err != nil {
		respondError(w, r, http.StatusForbidden, err.Error())
		return
	}

	referral, err := s.referralService.ActivateReferral(r.Context(), req.Code, req.ReferredOrgID, req.ReferredEmail)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, referral)
}

func (s *Server) handleListReferrals(w http.ResponseWriter, r *http.Request) {
	if s.referralService == nil {
		respondError(w, r, http.StatusNotImplemented, "referral service not configured")
		return
	}

	orgID, ok := s.resolveUsageOrgID(w, r)
	if !ok {
		return
	}

	referrals, err := s.referralService.ListReferrals(r.Context(), orgID)
	if err != nil {
		slog.Error("failed to list referrals", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to list referrals")
		return
	}

	if referrals == nil {
		referrals = []billing.Referral{}
	}

	respondJSON(w, http.StatusOK, referrals)
}

func (s *Server) handleCheckOrgLimit(w http.ResponseWriter, r *http.Request) {
	// Org limit checks are an internal-only operation; API key callers must not
	// be able to probe arbitrary user_ids.
	if scopesFromContext(r.Context()) != nil {
		respondError(w, r, http.StatusForbidden, "org limit check requires internal secret")
		return
	}

	if s.billingEnforcer == nil {
		respondJSON(w, http.StatusOK, map[string]string{"status": "allowed"})
		return
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		respondError(w, r, http.StatusBadRequest, "user_id query parameter is required")
		return
	}

	planTier := domain.PlanTier(r.URL.Query().Get("plan_tier"))
	if planTier == "" {
		planTier = domain.PlanFree
	}

	if err := s.billingEnforcer.CheckOrgCreationLimit(r.Context(), userID, planTier); err != nil {
		var le *billing.LimitError
		if errors.As(err, &le) {
			respondError(w, r, http.StatusForbidden, le)
			return
		}
		slog.Error("failed to check org creation limit", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to check org creation limit")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "allowed"})
}

// parseDateRange extracts from/to query parameters as dates.
func parseDateRange(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" || toStr == "" {
		respondError(w, r, http.StatusBadRequest, "from and to query parameters are required (format: YYYY-MM-DD)")
		return time.Time{}, time.Time{}, false
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid from date format (expected YYYY-MM-DD)")
		return time.Time{}, time.Time{}, false
	}

	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, "invalid to date format (expected YYYY-MM-DD)")
		return time.Time{}, time.Time{}, false
	}

	if to.Before(from) {
		respondError(w, r, http.StatusBadRequest, "to date must be after from date")
		return time.Time{}, time.Time{}, false
	}

	return from, to, true
}
