package api

import (
	"net/http"
	"time"

	"strait/internal/domain"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListOrgRuns(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if orgID == "" {
		respondError(w, r, http.StatusBadRequest, "org_id is required")
		return
	}

	// Enforce authorization: caller must have org-scoped key for this org
	// or use internal secret auth.
	callerOrgID := orgIDFromContext(r.Context())
	if callerOrgID == "" {
		// Not an org-scoped key -- check if this is an internal secret request.
		// Internal secret auth does not set scopes, so scopesFromContext returns nil.
		if scopesFromContext(r.Context()) != nil {
			respondError(w, r, http.StatusForbidden, "org-scoped api key required for cross-project queries")
			return
		}
	} else if callerOrgID != orgID {
		respondError(w, r, http.StatusForbidden, "api key does not belong to this organization")
		return
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	runs, err := s.store.ListRunsByOrg(r.Context(), orgID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list runs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(runs, limit, func(run domain.JobRun) string {
		return run.CreatedAt.Format(time.RFC3339Nano)
	}))
}

func (s *Server) handleListOrgJobs(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if orgID == "" {
		respondError(w, r, http.StatusBadRequest, "org_id is required")
		return
	}

	// Enforce authorization: caller must have org-scoped key for this org
	// or use internal secret auth.
	callerOrgID := orgIDFromContext(r.Context())
	if callerOrgID == "" {
		if scopesFromContext(r.Context()) != nil {
			respondError(w, r, http.StatusForbidden, "org-scoped api key required for cross-project queries")
			return
		}
	} else if callerOrgID != orgID {
		respondError(w, r, http.StatusForbidden, "api key does not belong to this organization")
		return
	}

	limit, cursor, err := parsePaginationParams(r)
	if err != nil {
		respondError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	jobs, err := s.store.ListJobsByOrg(r.Context(), orgID, limit+1, cursor)
	if err != nil {
		respondError(w, r, http.StatusInternalServerError, "failed to list jobs")
		return
	}

	respondJSON(w, http.StatusOK, paginatedResult(jobs, limit, func(job domain.Job) string {
		return job.CreatedAt.Format(time.RFC3339Nano)
	}))
}
