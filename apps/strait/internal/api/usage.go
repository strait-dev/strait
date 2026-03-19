package api

import (
	"log/slog"
	"net/http"
)

func (s *Server) handleGetCurrentUsage(w http.ResponseWriter, r *http.Request) {
	if s.usageService == nil {
		respondError(w, r, http.StatusNotImplemented, "usage service not configured")
		return
	}

	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		respondError(w, r, http.StatusBadRequest, "org_id query parameter is required")
		return
	}

	// Enforce tenant isolation for API key callers: resolve the org from the
	// caller's project and reject if it does not match the requested org_id.
	if scopesFromContext(r.Context()) != nil {
		projectID := projectIDFromContext(r.Context())
		if projectID == "" || s.billingEnforcer == nil {
			respondError(w, r, http.StatusForbidden, "cannot determine org for this API key")
			return
		}
		callerOrg, err := s.billingEnforcer.GetProjectOrgID(r.Context(), projectID)
		if err != nil {
			slog.Error("failed to resolve org for project", "project_id", projectID, "error", err)
			respondError(w, r, http.StatusInternalServerError, "failed to resolve org")
			return
		}
		if callerOrg != orgID {
			respondError(w, r, http.StatusForbidden, "org_id does not match the API key's organization")
			return
		}
	}

	// Count projects and members for this org.
	projects, err := s.store.ListProjectsByOrg(r.Context(), orgID)
	if err != nil {
		slog.Error("failed to list projects for usage", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to get usage data")
		return
	}

	// Member count not easily available without org-level store; pass 0 for now.
	usage, err := s.usageService.GetCurrentUsage(r.Context(), orgID, len(projects), 0)
	if err != nil {
		slog.Error("failed to get current usage", "error", err)
		respondError(w, r, http.StatusInternalServerError, "failed to get usage data")
		return
	}

	respondJSON(w, http.StatusOK, usage)
}
